// Package llm defines the provider-agnostic contract that the agent loop speaks.
//
// Every backend — Anthropic, OpenAI, OpenRouter, Ollama, or anything else —
// is reduced to a single interface: [Provider]. The rest of the system never
// imports a vendor SDK; it only ever sees the neutral types declared here.
// Adding a new backend means implementing one method, not touching the loop.
package llm

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a [Message].
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in a conversation. It is deliberately a superset of what
// any single provider needs so that history can be stored once and translated
// per-provider at request time.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content,omitempty"`

	// ToolCalls is set on assistant messages that request tool execution.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// The fields below are set on RoleTool messages that carry a tool result.
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ToolCall is a model's request to invoke a tool with the given JSON arguments.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// ToolSpec advertises a tool to the model: a name, a human description, and a
// JSON Schema object describing its parameters.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request is a single completion request. Providers translate it into their own
// wire format.
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolSpec
	Temperature float64
	MaxTokens   int
}

// EventKind discriminates the streamed [Event] union.
type EventKind int

const (
	// EventText carries an incremental chunk of assistant text.
	EventText EventKind = iota
	// EventToolCall carries one fully-assembled tool call.
	EventToolCall
	// EventDone is emitted exactly once when the turn completes cleanly.
	EventDone
	// EventError carries a terminal error; no further events follow.
	EventError
)

// Event is one item in a provider's response stream. Providers assemble partial
// tool-call deltas internally and only surface complete [ToolCall]s, keeping the
// agent loop free of wire-format bookkeeping.
type Event struct {
	Kind     EventKind
	Text     string
	ToolCall *ToolCall
	Usage    *Usage
	Err      error
}

// Usage reports token accounting for a turn, when the provider supplies it.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Provider is the single seam between the agent and any model backend.
//
// Stream must return a channel that yields [Event]s and is closed when the turn
// ends. Implementations are responsible for honoring ctx cancellation.
type Provider interface {
	// Name returns a short, stable identifier (e.g. "anthropic", "openai").
	Name() string
	// Stream issues req and returns a channel of streamed events.
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}
