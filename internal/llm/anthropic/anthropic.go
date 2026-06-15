// Package anthropic implements the llm.Provider contract against the Anthropic
// Messages API. Unlike the OpenAI format, Anthropic models a turn as an ordered
// list of typed content blocks (text, tool_use, tool_result), so this package
// translates the agent's flat history into and out of that block model.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nuu-maan/cogent/internal/llm"
	"github.com/Nuu-maan/cogent/internal/transport"
)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	apiVersion       = "2023-06-01"
	defaultMaxTokens = 4096
)

func init() {
	llm.Register("anthropic", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("anthropic: an API key is required")
		}
		base := cfg.BaseURL
		if base == "" {
			base = defaultBaseURL
		}
		return &Client{
			baseURL: base,
			apiKey:  cfg.APIKey,
			http:    transport.NewClient(cfg.HTTPTimeoutSeconds),
		}, nil
	})
}

// Client is the Anthropic Messages API provider.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// Name implements llm.Provider.
func (c *Client) Name() string { return "anthropic" }

// Stream implements llm.Provider.
func (c *Client) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	body, err := json.Marshal(c.buildBody(req))
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("anthropic: %s", transport.ReadError(resp))
	}

	out := make(chan llm.Event)
	go c.pump(resp, out)
	return out, nil
}

// pump translates Anthropic's SSE block protocol into llm.Events.
func (c *Client) pump(resp *http.Response, out chan<- llm.Event) {
	defer resp.Body.Close()
	defer close(out)

	type block struct {
		typ  string
		id   string
		name string
		json bytes.Buffer
	}
	blocks := map[int]*block{}
	usage := llm.Usage{}

	err := transport.ScanSSE(resp.Body, func(f transport.Frame) error {
		var ev streamEvent
		if err := json.Unmarshal([]byte(f.Data), &ev); err != nil {
			return fmt.Errorf("anthropic: decode event: %w", err)
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil && ev.Message.Usage != nil {
				usage.InputTokens = ev.Message.Usage.InputTokens
			}
		case "content_block_start":
			b := &block{}
			if ev.ContentBlock != nil {
				b.typ = ev.ContentBlock.Type
				b.id = ev.ContentBlock.ID
				b.name = ev.ContentBlock.Name
			}
			blocks[ev.Index] = b
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil || ev.Delta == nil {
				return nil
			}
			switch ev.Delta.Type {
			case "text_delta":
				out <- llm.Event{Kind: llm.EventText, Text: ev.Delta.Text}
			case "input_json_delta":
				b.json.WriteString(ev.Delta.PartialJSON)
			}
		case "content_block_stop":
			b := blocks[ev.Index]
			if b == nil || b.typ != "tool_use" {
				return nil
			}
			args := b.json.Bytes()
			if len(args) == 0 {
				args = []byte("{}")
			}
			out <- llm.Event{Kind: llm.EventToolCall, ToolCall: &llm.ToolCall{
				ID:   b.id,
				Name: b.name,
				Args: append(json.RawMessage(nil), args...),
			}}
		case "message_delta":
			if ev.Usage != nil {
				usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			// handled after the loop
		}
		return nil
	})
	if err != nil {
		out <- llm.Event{Kind: llm.EventError, Err: err}
		return
	}
	u := usage
	out <- llm.Event{Kind: llm.EventDone, Usage: &u}
}

func (c *Client) buildBody(req llm.Request) messagesRequest {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	var tools []wireTool
	for _, t := range req.Tools {
		tools = append(tools, wireTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	return messagesRequest{
		Model:       req.Model,
		System:      req.System,
		Messages:    toWireMessages(req.Messages),
		Tools:       tools,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}
}

// toWireMessages collapses the agent's flat history into Anthropic's
// alternating user/assistant turns. Tool results map to user-role blocks, and
// adjacent same-role messages are merged so the API's alternation rule holds
// even when one assistant turn issued several tool calls.
func toWireMessages(msgs []llm.Message) []wireMessage {
	var out []wireMessage
	appendBlocks := func(role string, blocks []wireBlock) {
		if len(blocks) == 0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content = append(out[n-1].Content, blocks...)
			return
		}
		out = append(out, wireMessage{Role: role, Content: blocks})
	}

	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			appendBlocks("user", []wireBlock{{Type: "text", Text: m.Content}})
		case llm.RoleTool:
			appendBlocks("user", []wireBlock{{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
				IsError:   m.IsError,
			}})
		case llm.RoleAssistant:
			var blocks []wireBlock
			if m.Content != "" {
				blocks = append(blocks, wireBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Args
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, wireBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			appendBlocks("assistant", blocks)
		}
	}
	return out
}

// --- wire types ---------------------------------------------------------------

type messagesRequest struct {
	Model       string        `json:"model"`
	System      string        `json:"system,omitempty"`
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

type wireBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type streamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Message *struct {
		Usage *struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
