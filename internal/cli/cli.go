// Package cli is the human-facing terminal frontend: a REPL that feeds input to
// the agent and a UI that renders streamed output. It is one of potentially many
// frontends — the agent core knows nothing about it.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Nuu-maan/cogent/internal/agent"
)

// UI is the terminal implementation of agent.UI. It is constructed independently
// of the REPL so it can be wired into the Agent before the REPL (which needs the
// Agent) exists, sidestepping the construction cycle.
type UI struct {
	out     io.Writer
	color   bool
	midText bool // true while assistant text is streaming on the current line
}

// NewUI returns a terminal UI writing to out. Colour is disabled when NO_COLOR
// is set, following the de-facto standard.
func NewUI(out io.Writer) *UI {
	return &UI{out: out, color: os.Getenv("NO_COLOR") == ""}
}

// REPL drives an interactive session against an Agent.
type REPL struct {
	agent *agent.Agent
	in    *bufio.Reader
	out   io.Writer
	ui    *UI
}

// NewREPL builds a REPL from an agent, its UI, and the I/O streams.
func NewREPL(a *agent.Agent, ui *UI, in io.Reader, out io.Writer) *REPL {
	return &REPL{agent: a, in: bufio.NewReader(in), out: out, ui: ui}
}

// Run loops reading input until EOF, /exit, or ctx cancellation.
func (r *REPL) Run(ctx context.Context) error {
	r.ui.banner()
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		fmt.Fprint(r.out, r.ui.paint(colorCyan, "\n› "))

		line, err := r.in.ReadString('\n')
		if err == io.EOF {
			fmt.Fprintln(r.out)
			return nil
		}
		if err != nil {
			return err
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			if r.command(input) {
				return nil
			}
			continue
		}
		if err := r.agent.Turn(ctx, input); err != nil {
			fmt.Fprintln(r.out, r.ui.paint(colorRed, "error: "+err.Error()))
		}
	}
}

// command handles slash commands and reports whether the REPL should exit.
func (r *REPL) command(input string) (quit bool) {
	switch strings.Fields(input)[0] {
	case "/exit", "/quit":
		return true
	case "/reset":
		r.agent.Session().Replace(nil)
		r.ui.Notice("conversation reset")
	case "/tokens":
		u := r.agent.Session().Usage()
		r.ui.Notice(fmt.Sprintf("tokens — input: %d, output: %d", u.InputTokens, u.OutputTokens))
	case "/help":
		fmt.Fprintln(r.out, helpText)
	default:
		r.ui.Notice("unknown command; try /help")
	}
	return false
}

const helpText = `commands:
  /help     show this help
  /tokens   show cumulative token usage
  /reset    clear the conversation history
  /exit     quit (Ctrl-D also works)`

// --- agent.UI implementation --------------------------------------------------

const (
	colorReset = "\x1b[0m"
	colorDim   = "\x1b[2m"
	colorCyan  = "\x1b[36m"
	colorGreen = "\x1b[32m"
	colorRed   = "\x1b[31m"
)

func (u *UI) paint(color, s string) string {
	if !u.color {
		return s
	}
	return color + s + colorReset
}

func (u *UI) banner() {
	fmt.Fprintln(u.out, u.paint(colorCyan, "cogent")+u.paint(colorDim, " — a small, hackable coding agent. /help for commands."))
}

// AssistantText implements agent.UI.
func (u *UI) AssistantText(chunk string) {
	u.midText = true
	fmt.Fprint(u.out, chunk)
}

// AssistantDone implements agent.UI.
func (u *UI) AssistantDone() { u.breakText() }

// ToolStart implements agent.UI.
func (u *UI) ToolStart(name string, args json.RawMessage) {
	u.breakText()
	fmt.Fprintln(u.out, u.paint(colorDim, "  ⚙ "+name+" "+compactJSON(args)))
}

// ToolEnd implements agent.UI.
func (u *UI) ToolEnd(name, result string, isErr bool) {
	mark, color := "✓", colorGreen
	if isErr {
		mark, color = "✗", colorRed
	}
	fmt.Fprintln(u.out, u.paint(color, "  "+mark+" ")+u.paint(colorDim, firstLine(result)))
}

// Notice implements agent.UI.
func (u *UI) Notice(msg string) {
	u.breakText()
	fmt.Fprintln(u.out, u.paint(colorDim, "  · "+msg))
}

// breakText ends an in-progress assistant line before printing out-of-band UI.
func (u *UI) breakText() {
	if u.midText {
		fmt.Fprintln(u.out)
		u.midText = false
	}
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return truncate(string(raw), 100)
	}
	return truncate(buf.String(), 100)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return truncate(s[:i], 120) + " …"
	}
	return truncate(s, 120)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
