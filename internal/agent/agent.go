// Package agent contains the control loop that turns a user message into model
// output and tool execution. The loop is intentionally small and readable: it
// orchestrates a provider, a tool registry, and a window manager, and owns none
// of their internals. Everything interesting is behind an interface so this file
// rarely needs to change.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nuu-maan/cogent/internal/llm"
	"github.com/Nuu-maan/cogent/internal/tool"
	"github.com/Nuu-maan/cogent/internal/window"
)

// defaultMaxIterations bounds how many model<->tool round-trips a single user
// turn may take, so a misbehaving model can't loop forever.
const defaultMaxIterations = 50

// Config is the immutable wiring for an Agent.
type Config struct {
	Provider    llm.Provider
	Model       string
	System      string
	Tools       *tool.Registry
	Window      *window.Manager
	UI          UI
	Temperature float64
	MaxTokens   int

	// MaxIterations caps tool round-trips per turn; zero uses the default.
	MaxIterations int
}

// Agent drives a conversation against a single provider and tool set.
type Agent struct {
	cfg     Config
	session *Session
}

// New constructs an Agent. The Session starts empty and persists across turns.
func New(cfg Config) *Agent {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = defaultMaxIterations
	}
	return &Agent{cfg: cfg, session: NewSession()}
}

// Session exposes the live conversation state (for inspection and persistence).
func (a *Agent) Session() *Session { return a.session }

// Turn runs one user turn to completion: it appends the user's input, then
// repeatedly calls the model and executes any requested tools until the model
// responds with no further tool calls. It returns when the turn is settled or
// ctx is cancelled.
func (a *Agent) Turn(ctx context.Context, userInput string) error {
	a.session.Append(llm.Message{Role: llm.RoleUser, Content: userInput})

	for iter := 0; iter < a.cfg.MaxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		a.maybeCompact(ctx)

		text, calls, usage, err := a.stream(ctx, a.buildRequest())
		if err != nil {
			return err
		}
		a.session.AddUsage(usage)
		a.session.Append(llm.Message{Role: llm.RoleAssistant, Content: text, ToolCalls: calls})

		// No tool calls means the model is done; hand control back to the user.
		if len(calls) == 0 {
			a.cfg.UI.AssistantDone()
			return nil
		}

		for _, call := range calls {
			result, isErr := a.runTool(ctx, call)
			a.session.Append(llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    result,
				IsError:    isErr,
			})
		}
	}

	a.cfg.UI.Notice(fmt.Sprintf("stopped after %d tool iterations", a.cfg.MaxIterations))
	return nil
}

// buildRequest snapshots the current session into a provider request.
func (a *Agent) buildRequest() llm.Request {
	return llm.Request{
		Model:       a.cfg.Model,
		System:      a.cfg.System,
		Messages:    a.session.Messages(),
		Tools:       a.cfg.Tools.Specs(),
		Temperature: a.cfg.Temperature,
		MaxTokens:   a.cfg.MaxTokens,
	}
}

// maybeCompact asks the window manager to shrink history when it overflows,
// degrading gracefully: a summarization failure is reported, not fatal.
func (a *Agent) maybeCompact(ctx context.Context) {
	if a.cfg.Window == nil {
		return
	}
	msgs, compacted, err := a.cfg.Window.Compact(ctx, a.session.Messages())
	if err != nil {
		a.cfg.UI.Notice("compaction skipped: " + err.Error())
		return
	}
	if compacted {
		a.session.Replace(msgs)
		a.cfg.UI.Notice("compacted earlier history to stay within the context window")
	}
}

// stream consumes one provider response, surfacing text to the UI live and
// collecting any tool calls for execution.
func (a *Agent) stream(ctx context.Context, req llm.Request) (string, []llm.ToolCall, *llm.Usage, error) {
	events, err := a.cfg.Provider.Stream(ctx, req)
	if err != nil {
		return "", nil, nil, err
	}

	var text strings.Builder
	var calls []llm.ToolCall
	var usage *llm.Usage

	for ev := range events {
		switch ev.Kind {
		case llm.EventText:
			text.WriteString(ev.Text)
			a.cfg.UI.AssistantText(ev.Text)
		case llm.EventToolCall:
			calls = append(calls, *ev.ToolCall)
		case llm.EventDone:
			usage = ev.Usage
		case llm.EventError:
			return text.String(), calls, usage, ev.Err
		}
	}
	return text.String(), calls, usage, nil
}

// runTool dispatches one tool call, reporting progress to the UI. Failures are
// returned in-band as error results so the model can read and recover from them.
func (a *Agent) runTool(ctx context.Context, call llm.ToolCall) (string, bool) {
	a.cfg.UI.ToolStart(call.Name, call.Args)

	t, ok := a.cfg.Tools.Get(call.Name)
	if !ok {
		msg := fmt.Sprintf("unknown tool %q", call.Name)
		a.cfg.UI.ToolEnd(call.Name, msg, true)
		return msg, true
	}

	result, err := t.Run(ctx, call.Args)
	if err != nil {
		a.cfg.UI.ToolEnd(call.Name, err.Error(), true)
		return err.Error(), true
	}
	a.cfg.UI.ToolEnd(call.Name, result, false)
	return result, false
}
