// Command cogent is a small, hackable coding agent: a single provider-agnostic
// loop wired to a built-in toolset and a terminal frontend. The binary's job is
// only composition — read config, construct the pieces, and run them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Nuu-maan/cogent/internal/agent"
	"github.com/Nuu-maan/cogent/internal/cli"
	"github.com/Nuu-maan/cogent/internal/config"
	"github.com/Nuu-maan/cogent/internal/llm"
	"github.com/Nuu-maan/cogent/internal/tool"
	"github.com/Nuu-maan/cogent/internal/window"

	// Providers self-register via their init functions; importing for side
	// effects is what makes them discoverable by name through llm.Open.
	_ "github.com/Nuu-maan/cogent/internal/llm/anthropic"
	_ "github.com/Nuu-maan/cogent/internal/llm/openai"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cogent: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath = flag.String("config", defaultConfigPath(), "path to config JSON file")
		workspace  = flag.String("workspace", "", "workspace root (overrides config)")
		provider   = flag.String("provider", "", "provider name (overrides config)")
		model      = flag.String("model", "", "model id (overrides config)")
		prompt     = flag.String("p", "", "run a single prompt non-interactively, then exit")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	// Command-line flags take precedence over file and environment.
	if *workspace != "" {
		cfg.Workspace = *workspace
	}
	if *provider != "" {
		cfg.Provider = *provider
	}
	if *model != "" {
		cfg.Model = *model
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Cancel cleanly on Ctrl-C / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Build the UI first so the agent can be wired with it; the REPL then wraps
	// both, avoiding an agent<->UI construction cycle.
	ui := cli.NewUI(os.Stdout)
	a, err := buildAgent(cfg, ui)
	if err != nil {
		return err
	}

	// One-shot mode for scripting and demos.
	if *prompt != "" {
		return a.Turn(ctx, *prompt)
	}

	return cli.NewREPL(a, ui, os.Stdin, os.Stdout).Run(ctx)
}

// buildAgent composes the provider, tools, window manager, and system prompt
// into a ready Agent. The UI is attached afterwards by the frontend.
func buildAgent(cfg config.Config, ui agent.UI) (*agent.Agent, error) {
	provider, err := llm.Open(cfg.Provider, llm.ProviderConfig{
		APIKey:             cfg.APIKey(),
		BaseURL:            cfg.BaseURL,
		HTTPTimeoutSeconds: cfg.HTTPTimeoutSeconds,
	})
	if err != nil {
		return nil, err
	}

	ws, err := tool.NewWorkspace(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	registry := tool.NewRegistry()
	tool.RegisterDefaults(registry, ws)

	summarizer := &window.ModelSummarizer{Provider: provider, Model: cfg.Context.SummaryModel}
	winmgr := window.NewManager(cfg.Context.MaxTokens, cfg.Context.KeepRecent, summarizer)

	system, err := systemPrompt(cfg, ws)
	if err != nil {
		return nil, err
	}

	return agent.New(agent.Config{
		Provider:    provider,
		Model:       cfg.Model,
		System:      system,
		Tools:       registry,
		Window:      winmgr,
		UI:          ui,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	}), nil
}

// systemPrompt assembles the system prompt: a base prompt (from config's
// system_prompt_file, a SYSTEM.md in the workspace, or a built-in default),
// then any project instructions found in AGENTS.md appended underneath.
func systemPrompt(cfg config.Config, ws *tool.Workspace) (string, error) {
	base := defaultSystem

	candidates := []string{cfg.SystemPromptFile, filepath.Join(ws.Root(), "SYSTEM.md")}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if b, err := os.ReadFile(p); err == nil {
			base = string(b)
			break
		}
	}

	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(base))
	if b, err := os.ReadFile(filepath.Join(ws.Root(), "AGENTS.md")); err == nil {
		sb.WriteString("\n\n# Project instructions (AGENTS.md)\n\n")
		sb.WriteString(strings.TrimSpace(string(b)))
	}
	sb.WriteString(fmt.Sprintf("\n\nThe workspace root is: %s", ws.Root()))
	return sb.String(), nil
}

const defaultSystem = `You are cogent, a focused coding agent operating inside a user's workspace.

Work in small, verifiable steps. Prefer reading before writing. Use the provided
tools to inspect and modify files and to run commands; never guess a file's
contents when you can read it. When you change code, keep edits minimal and in
the style of the surrounding code. Explain what you did concisely after acting,
not before. If a task is ambiguous, ask one clarifying question rather than
assuming.`

func defaultConfigPath() string {
	if dir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(dir, ".cogent", "config.json")
	}
	return "cogent.json"
}
