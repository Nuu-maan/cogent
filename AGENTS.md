# Project instructions for cogent

These notes are appended to the agent's system prompt at startup. Keep them
short and concrete — they are spent context on every turn.

## Conventions

- Standard library only. Do not add third-party dependencies without discussion.
- Every exported type and function has a doc comment. Match the existing voice:
  explain *why*, not just *what*.
- New backends go in `internal/llm/<name>/` and self-register via `init`.
- New tools implement `tool.Tool` and are wired in `internal/tool/builtin.go`.
- Keep the agent loop (`internal/agent/agent.go`) free of provider- or
  tool-specific logic. If something leaks in, it belongs behind an interface.

## Before you finish

- `go build ./...` and `go test ./...` must pass.
- `gofmt -l .` must report no files.
