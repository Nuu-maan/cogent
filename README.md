# cogent

A small, hackable coding agent in Go. One provider-agnostic loop, a handful of
tools, and a context-window manager you can actually read — under ~2,000 lines,
zero third-party dependencies.

> The good coding agents feel good because the **model** is good. The loop around
> it is simple. cogent makes that loop small enough to read in an afternoon and
> swap any part of: the model, the tools, or how the context window is managed.

```
$ cogent
cogent — a small, hackable coding agent. /help for commands.

› add a /version flag to the CLI and wire it up
  ⚙ grep {"pattern":"flag.String"}
  ✓ cmd/cogent/main.go:31:configPath = flag.String(...
  ⚙ edit_file {"path":"cmd/cogent/main.go", ...}
  ✓ replaced 1 occurrence(s) in cmd/cogent/main.go
Done — added a `-version` flag that prints the build version and exits.
```

## Why another agent

Claude Code, Pi, and friends are all the same shape underneath: a loop that
sends context to a model, runs the tools the model asks for, and feeds the
results back. cogent keeps that shape **visible**. There is no plugin framework
to learn before you can see how a tool call becomes a file edit.

- **Provider-agnostic.** Anthropic, OpenAI, OpenRouter, and Ollama out of the
  box. Adding another backend is one file implementing one interface.
- **Context engineering is first-class.** Compaction lives behind an interface;
  the default is summarize-and-keep-recent, but topic-based or code-aware
  strategies are a drop-in.
- **Zero dependencies.** Standard library only. The whole thing builds in a
  second and the supply chain is `go`.
- **Sandboxed by default.** Filesystem tools are confined to a workspace root.

## Quick start

```bash
# 1. Build (Go 1.22+)
go build -o cogent ./cmd/cogent

# 2. Point it at a model
export ANTHROPIC_API_KEY=sk-ant-...
./cogent -model claude-sonnet-4-6

# …or run anything OpenAI-compatible
export OPENROUTER_API_KEY=sk-or-...
./cogent -provider openrouter -model anthropic/claude-sonnet-4-6

# …or fully local, no key
ollama serve &
./cogent -provider ollama -model qwen2.5-coder

# One-shot, non-interactive
./cogent -p "summarize internal/agent/agent.go"
```

Keys can also live in a `.env` file in the working directory (copy
`.env.example`). A generic `API-KEY=` entry works for whichever provider is
selected; real environment variables take precedence over `.env`.

## How it works

The entire agent is this loop, in `internal/agent/agent.go`:

```
        user input
            │
            ▼
   ┌──────────────────┐   compact if the window is full
   │  window.Manager  │◀──────────────────────────────┐
   └──────────────────┘                                │
            │ messages                                 │
            ▼                                           │
   ┌──────────────────┐   stream of text + tool calls  │
   │   llm.Provider   │────────────────────────────────┤
   └──────────────────┘                                │
            │ tool calls                                │
            ▼                                           │
   ┌──────────────────┐   results appended to history  │
   │  tool.Registry   │────────────────────────────────┘
   └──────────────────┘
            │ no more tool calls
            ▼
       back to user
```

Three interfaces are the whole contract:

| Interface          | File                         | Responsibility                          |
|--------------------|------------------------------|-----------------------------------------|
| `llm.Provider`     | `internal/llm/llm.go`        | Turn a request into a stream of events  |
| `tool.Tool`        | `internal/tool/tool.go`      | A capability the model can call         |
| `window.Summarizer`| `internal/window/window.go`  | Condense history when the window fills   |

Everything else is wiring.

## Project layout

```
cogent/
├── cmd/cogent/            # entrypoint — composition only
└── internal/
    ├── agent/             # the loop, session state, UI seam
    ├── llm/               # provider contract + driver registry
    │   ├── anthropic/     # native Messages API backend
    │   └── openai/        # OpenAI-compatible backend (OpenAI/OpenRouter/Ollama)
    ├── tool/              # tool interface, registry, workspace sandbox, builtins
    ├── window/            # context-window estimation + compaction
    ├── transport/         # shared HTTP + SSE plumbing
    ├── config/            # JSON + env configuration
    └── cli/               # terminal REPL frontend
```

## Extending it

**Add a tool** — implement four methods and register it:

```go
type Now struct{}

func (Now) Name() string                 { return "now" }
func (Now) Description() string           { return "Return the current time." }
func (Now) Schema() json.RawMessage       { return json.RawMessage(`{"type":"object"}`) }
func (Now) Run(context.Context, json.RawMessage) (string, error) {
    return time.Now().Format(time.RFC3339), nil
}

// in main: registry.Register(Now{})
```

**Add a provider** — implement `llm.Provider` and self-register:

```go
func init() {
    llm.Register("myllm", func(cfg llm.ProviderConfig) (llm.Provider, error) {
        return &Client{key: cfg.APIKey}, nil
    })
}
```

Then blank-import the package in `main.go` and it's selectable with
`-provider myllm`. This is the `database/sql` driver pattern.

**Change context strategy** — implement `window.Summarizer` (or replace the
`Manager` entirely) to do RAG, topic clustering, or code-aware compaction.

## Configuration

Flags override environment, which overrides `~/.cogent/config.json`. A
`SYSTEM.md` or `AGENTS.md` in the workspace is folded into the system prompt at
startup, like Pi. See `config.example.json`.

## Status

This is a teaching-grade core that really runs, not a Claude Code replacement.
It streams, calls tools, compacts context, and sandboxes the filesystem. It does
not (yet) do sub-agents, MCP, or prompt caching — all of which fit cleanly behind
the existing interfaces. PRs welcome.

## License

MIT — see [LICENSE](LICENSE).
