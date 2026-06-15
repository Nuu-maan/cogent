package window

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nuu-maan/cogent/internal/llm"
)

const summarySystem = "You compress conversation history for an autonomous coding agent. " +
	"Preserve concrete facts the agent will still need: file paths, decisions made, " +
	"the user's goal, and any unfinished work. Drop pleasantries and superseded detail. " +
	"Write a tight, factual summary in plain prose."

// ModelSummarizer condenses history by asking a model to summarize a rendered
// transcript. A small, cheap model is a fine choice here and keeps compaction
// from competing for the main model's budget.
type ModelSummarizer struct {
	Provider llm.Provider
	Model    string
}

// Summarize implements Summarizer.
func (s *ModelSummarizer) Summarize(ctx context.Context, msgs []llm.Message) (string, error) {
	req := llm.Request{
		Model:       s.Model,
		System:      summarySystem,
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: renderTranscript(msgs)}},
		MaxTokens:   1024,
		Temperature: 0,
	}
	text, _, err := llm.Complete(ctx, s.Provider, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// renderTranscript flattens structured history into a readable transcript for
// the summarizer to work from.
func renderTranscript(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			fmt.Fprintf(&b, "USER: %s\n", m.Content)
		case llm.RoleAssistant:
			if m.Content != "" {
				fmt.Fprintf(&b, "ASSISTANT: %s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "ASSISTANT called %s(%s)\n", tc.Name, string(tc.Args))
			}
		case llm.RoleTool:
			fmt.Fprintf(&b, "TOOL %s -> %s\n", m.ToolName, truncateRunes(m.Content, 400))
		}
	}
	return b.String()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
