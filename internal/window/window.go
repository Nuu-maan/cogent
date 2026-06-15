// Package window manages the context window: how much history fits, and what to
// do when it overflows. This is the lever Pi-style "context engineering" is
// really about — controlling exactly what the model sees each turn. The default
// strategy is summarize-and-keep-recent, but Compactor is an interface so a
// project can drop in topic-based or code-aware compaction without touching the
// agent loop.
package window

import (
	"context"
	"fmt"

	"github.com/Nuu-maan/cogent/internal/llm"
)

// EstimateTokens approximates the token cost of a message slice. It deliberately
// avoids a tokenizer dependency: ~4 characters per token is close enough to
// drive a compaction threshold, and being provider-agnostic matters more here
// than exactness.
func EstimateTokens(msgs []llm.Message) int {
	const charsPerToken = 4
	chars := 0
	for _, m := range msgs {
		chars += len(m.Content) + len(m.ToolName)
		for _, tc := range m.ToolCalls {
			chars += len(tc.Name) + len(tc.Args)
		}
		chars += 8 // per-message structural overhead
	}
	return chars / charsPerToken
}

// Summarizer condenses a slice of messages into a single paragraph. The default
// implementation calls a model; tests and offline modes can substitute a stub.
type Summarizer interface {
	Summarize(ctx context.Context, msgs []llm.Message) (string, error)
}

// Manager applies a compaction policy to a conversation.
type Manager struct {
	// MaxTokens is the estimated budget at which compaction triggers.
	MaxTokens int
	// KeepRecent is the minimum number of trailing messages preserved verbatim.
	KeepRecent int
	// Summarizer produces the replacement summary for compacted history.
	Summarizer Summarizer
}

// NewManager returns a Manager with sensible defaults applied to zero fields.
func NewManager(maxTokens, keepRecent int, s Summarizer) *Manager {
	if maxTokens <= 0 {
		maxTokens = 24000
	}
	if keepRecent <= 0 {
		keepRecent = 6
	}
	return &Manager{MaxTokens: maxTokens, KeepRecent: keepRecent, Summarizer: s}
}

// Compact returns history unchanged when it fits the budget. Otherwise it
// summarizes the older portion and returns [summary] + recent tail. The second
// result reports whether compaction occurred.
func (m *Manager) Compact(ctx context.Context, msgs []llm.Message) ([]llm.Message, bool, error) {
	if EstimateTokens(msgs) <= m.MaxTokens || len(msgs) <= m.KeepRecent || m.Summarizer == nil {
		return msgs, false, nil
	}

	cut := boundary(msgs, len(msgs)-m.KeepRecent)
	if cut <= 0 {
		return msgs, false, nil // nothing safely summarizable yet
	}
	head, tail := msgs[:cut], msgs[cut:]

	summary, err := m.Summarizer.Summarize(ctx, head)
	if err != nil {
		return msgs, false, fmt.Errorf("window: summarize: %w", err)
	}

	out := make([]llm.Message, 0, len(tail)+1)
	out = append(out, llm.Message{
		Role:    llm.RoleUser,
		Content: "Summary of the earlier conversation so far:\n\n" + summary,
	})
	out = append(out, tail...)
	return out, true, nil
}

// boundary nudges a proposed cut index forward to the next user message so the
// surviving tail never begins with an orphaned tool result whose originating
// tool call was summarized away — a constraint block-based APIs enforce.
func boundary(msgs []llm.Message, idx int) int {
	if idx < 0 {
		idx = 0
	}
	for i := idx; i < len(msgs); i++ {
		if msgs[i].Role == llm.RoleUser {
			return i
		}
	}
	return idx
}
