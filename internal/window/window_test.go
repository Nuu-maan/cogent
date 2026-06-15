package window

import (
	"context"
	"strings"
	"testing"

	"github.com/Nuu-maan/cogent/internal/llm"
)

// stubSummarizer returns a fixed summary and records that it ran.
type stubSummarizer struct{ called bool }

func (s *stubSummarizer) Summarize(_ context.Context, _ []llm.Message) (string, error) {
	s.called = true
	return "SUMMARY", nil
}

func TestEstimateTokensGrowsWithContent(t *testing.T) {
	small := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	big := []llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("x", 4000)}}
	if EstimateTokens(small) >= EstimateTokens(big) {
		t.Fatalf("expected larger content to estimate more tokens")
	}
}

func TestCompactNoOpUnderBudget(t *testing.T) {
	m := NewManager(10000, 4, &stubSummarizer{})
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "short"}}
	out, compacted, err := m.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if compacted {
		t.Fatal("did not expect compaction under budget")
	}
	if len(out) != len(msgs) {
		t.Fatalf("history changed unexpectedly: %d != %d", len(out), len(msgs))
	}
}

func TestCompactSummarizesAndKeepsRecent(t *testing.T) {
	stub := &stubSummarizer{}
	// Tiny budget forces compaction; keep the last 2 messages verbatim.
	m := NewManager(1, 2, stub)

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: strings.Repeat("a", 100)},
		{Role: llm.RoleAssistant, Content: strings.Repeat("b", 100)},
		{Role: llm.RoleUser, Content: strings.Repeat("c", 100)},
		{Role: llm.RoleAssistant, Content: "recent reply"},
	}
	out, compacted, err := m.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if !compacted {
		t.Fatal("expected compaction over budget")
	}
	if !stub.called {
		t.Fatal("summarizer was not invoked")
	}
	if out[0].Role != llm.RoleUser || !strings.Contains(out[0].Content, "SUMMARY") {
		t.Fatalf("first message should carry the summary, got %+v", out[0])
	}
	// The most recent message must survive verbatim.
	last := out[len(out)-1]
	if last.Content != "recent reply" {
		t.Fatalf("recent message not preserved: %q", last.Content)
	}
}

func TestBoundaryLandsOnUserMessage(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser},
		{Role: llm.RoleAssistant},
		{Role: llm.RoleTool},
		{Role: llm.RoleUser},
		{Role: llm.RoleAssistant},
	}
	// Proposing a cut at the tool message must advance to the next user message.
	if got := boundary(msgs, 2); msgs[got].Role != llm.RoleUser {
		t.Fatalf("boundary landed on %s, want user", msgs[got].Role)
	}
}
