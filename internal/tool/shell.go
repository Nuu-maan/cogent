package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	defaultShellTimeout = 120 * time.Second
	maxShellOutput      = 64 << 10 // 64 KiB of combined output is plenty.
)

// Shell runs a command through the platform shell, rooted at the workspace.
type Shell struct{ WS *Workspace }

func (t *Shell) Name() string { return "run_shell" }
func (t *Shell) Description() string {
	return "Run a shell command in the workspace and return its combined stdout/stderr. Uses PowerShell on Windows and sh elsewhere."
}
func (t *Shell) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The command line to execute."},
			"timeout_seconds": {"type": "integer", "description": "Optional timeout; defaults to 120s."}
		},
		"required": ["command"]
	}`)
}

func (t *Shell) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("command is empty")
	}

	timeout := defaultShellTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, cmdArgs := shellCommand(args.Command)
	cmd := exec.CommandContext(ctx, name, cmdArgs...)
	cmd.Dir = t.WS.Root()

	out, err := cmd.CombinedOutput()
	text := truncate(string(out), maxShellOutput)

	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil {
		// Return output alongside the error so the model sees the failure detail.
		if text == "" {
			return "", fmt.Errorf("command failed: %w", err)
		}
		return text, fmt.Errorf("command exited with error: %w", err)
	}
	if text == "" {
		return "(no output)", nil
	}
	return text, nil
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", command}
	}
	return "sh", []string{"-c", command}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n... [truncated %d bytes]", len(s)-max)
}
