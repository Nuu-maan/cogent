// Package openai implements the llm.Provider contract against the OpenAI
// Chat Completions API. Because that wire format is a de-facto standard, the
// same code serves OpenAI, OpenRouter, and a local Ollama server — they differ
// only in base URL and whether an API key is required.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/Nuu-maan/cogent/internal/llm"
	"github.com/Nuu-maan/cogent/internal/transport"
)

func init() {
	llm.Register("openai", factory("openai", "https://api.openai.com/v1", true))
	llm.Register("openrouter", factory("openrouter", "https://openrouter.ai/api/v1", true))
	llm.Register("ollama", factory("ollama", "http://localhost:11434/v1", false))
}

func factory(name, defaultBaseURL string, requireKey bool) llm.Factory {
	return func(cfg llm.ProviderConfig) (llm.Provider, error) {
		if requireKey && cfg.APIKey == "" {
			return nil, fmt.Errorf("openai: provider %q requires an API key", name)
		}
		base := cfg.BaseURL
		if base == "" {
			base = defaultBaseURL
		}
		return &Client{
			name:    name,
			baseURL: base,
			apiKey:  cfg.APIKey,
			http:    transport.NewClient(cfg.HTTPTimeoutSeconds),
		}, nil
	}
}

// Client is an OpenAI-compatible provider bound to a single base URL.
type Client struct {
	name    string
	baseURL string
	apiKey  string
	http    *http.Client
}

// Name implements llm.Provider.
func (c *Client) Name() string { return c.name }

// Stream implements llm.Provider.
func (c *Client) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	body, err := json.Marshal(c.buildBody(req))
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("openai: %s", transport.ReadError(resp))
	}

	out := make(chan llm.Event)
	go c.pump(resp, out)
	return out, nil
}

// pump translates the SSE response into llm.Events and closes out when done.
func (c *Client) pump(resp *http.Response, out chan<- llm.Event) {
	defer resp.Body.Close()
	defer close(out)

	// Tool-call arguments arrive as fragments keyed by their position index.
	type acc struct {
		id   string
		name string
		args bytes.Buffer
	}
	tools := map[int]*acc{}
	var usage *llm.Usage

	emitErr := func(err error) { out <- llm.Event{Kind: llm.EventError, Err: err} }

	err := transport.ScanSSE(resp.Body, func(f transport.Frame) error {
		if f.Data == "[DONE]" {
			return nil
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(f.Data), &chunk); err != nil {
			return fmt.Errorf("openai: decode chunk: %w", err)
		}
		if chunk.Usage != nil {
			usage = &llm.Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
		}
		for _, choice := range chunk.Choices {
			if t := choice.Delta.Content; t != "" {
				out <- llm.Event{Kind: llm.EventText, Text: t}
			}
			for _, tc := range choice.Delta.ToolCalls {
				a := tools[tc.Index]
				if a == nil {
					a = &acc{}
					tools[tc.Index] = a
				}
				if tc.ID != "" {
					a.id = tc.ID
				}
				if tc.Function.Name != "" {
					a.name = tc.Function.Name
				}
				a.args.WriteString(tc.Function.Arguments)
			}
		}
		return nil
	})
	if err != nil {
		emitErr(err)
		return
	}

	// Emit assembled tool calls in stable index order.
	indexes := make([]int, 0, len(tools))
	for i := range tools {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		a := tools[i]
		args := a.args.Bytes()
		if len(args) == 0 {
			args = []byte("{}")
		}
		out <- llm.Event{Kind: llm.EventToolCall, ToolCall: &llm.ToolCall{
			ID:   a.id,
			Name: a.name,
			Args: append(json.RawMessage(nil), args...),
		}}
	}

	out <- llm.Event{Kind: llm.EventDone, Usage: usage}
}

func (c *Client) buildBody(req llm.Request) chatRequest {
	msgs := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, toWireMessage(m))
	}

	var tools []chatTool
	for _, t := range req.Tools {
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	body := chatRequest{
		Model:       req.Model,
		Messages:    msgs,
		Tools:       tools,
		Stream:      true,
		Temperature: req.Temperature,
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	// Ask for usage on the final chunk where supported; harmless elsewhere.
	body.StreamOptions = &streamOptions{IncludeUsage: true}
	return body
}

func toWireMessage(m llm.Message) chatMessage {
	switch m.Role {
	case llm.RoleTool:
		return chatMessage{Role: "tool", ToolCallID: m.ToolCallID, Content: m.Content}
	case llm.RoleAssistant:
		cm := chatMessage{Role: "assistant", Content: m.Content}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, wireToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: wireToolCallFunc{Name: tc.Name, Arguments: string(tc.Args)},
			})
		}
		return cm
	default:
		return chatMessage{Role: string(m.Role), Content: m.Content}
	}
}

// --- wire types ---------------------------------------------------------------

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	Tools         []chatTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	Temperature   float64        `json:"temperature"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireToolCallFunc `json:"function"`
}

type wireToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatToolFunc `json:"function"`
}

type chatToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
