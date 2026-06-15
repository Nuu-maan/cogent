package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxSearchMatches bounds output so a broad pattern can't flood the context.
const maxSearchMatches = 200

// skipDirs are never descended into during a search.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".cache":       true,
}

// Grep searches file contents under a workspace path for a regular expression.
type Grep struct{ WS *Workspace }

func (t *Grep) Name() string { return "grep" }
func (t *Grep) Description() string {
	return "Recursively search workspace file contents for a Go-syntax regular expression. Returns path:line:text matches."
}
func (t *Grep) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "RE2 (Go) regular expression to search for."},
			"path": {"type": "string", "description": "Directory or file to search under, relative to the workspace root. Defaults to the root."}
		},
		"required": ["pattern"]
	}`)
}

func (t *Grep) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return "", err
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regular expression: %w", err)
	}
	root, err := t.WS.Resolve(args.Path)
	if err != nil {
		return "", err
	}

	var matches []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than abort the whole search
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= maxSearchMatches {
			return filepath.SkipAll
		}
		scanFile(path, t.WS.Rel(path), re, &matches)
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}

	if len(matches) == 0 {
		return "no matches", nil
	}
	out := strings.Join(matches, "\n")
	if len(matches) >= maxSearchMatches {
		out += fmt.Sprintf("\n... [stopped at %d matches]", maxSearchMatches)
	}
	return out, nil
}

func scanFile(abs, display string, re *regexp.Regexp, matches *[]string) {
	b, err := os.ReadFile(abs)
	if err != nil || isBinary(b) {
		return
	}
	for i, line := range strings.Split(string(b), "\n") {
		if len(*matches) >= maxSearchMatches {
			return
		}
		if re.MatchString(line) {
			*matches = append(*matches, fmt.Sprintf("%s:%d:%s", display, i+1, strings.TrimSpace(line)))
		}
	}
}

// isBinary uses the same heuristic as common search tools: a NUL byte in the
// first 8 KiB means "not text".
func isBinary(b []byte) bool {
	if len(b) > 8<<10 {
		b = b[:8<<10]
	}
	return bytes.IndexByte(b, 0) != -1
}
