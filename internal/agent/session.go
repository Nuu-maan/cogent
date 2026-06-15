package agent

import "github.com/Nuu-maan/cogent/internal/llm"

// Session is the mutable conversation state for one agent run: an ordered list
// of messages plus running token accounting. It is the single source of truth
// the window manager compacts and the provider is fed from.
type Session struct {
	messages []llm.Message
	usage    llm.Usage
}

// NewSession returns an empty session.
func NewSession() *Session { return &Session{} }

// Append adds a message to the history.
func (s *Session) Append(m llm.Message) { s.messages = append(s.messages, m) }

// Messages returns the current history. The slice is owned by the Session;
// callers must not mutate it, only read or pass it on.
func (s *Session) Messages() []llm.Message { return s.messages }

// Replace swaps the history wholesale, as compaction does.
func (s *Session) Replace(msgs []llm.Message) { s.messages = msgs }

// Len reports the number of messages in history.
func (s *Session) Len() int { return len(s.messages) }

// AddUsage accumulates token usage reported by the provider.
func (s *Session) AddUsage(u *llm.Usage) {
	if u == nil {
		return
	}
	s.usage.InputTokens += u.InputTokens
	s.usage.OutputTokens += u.OutputTokens
}

// Usage returns cumulative token usage for the session.
func (s *Session) Usage() llm.Usage { return s.usage }
