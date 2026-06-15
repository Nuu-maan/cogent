package tool

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Workspace confines filesystem tools to a single root directory. Every path a
// tool receives is resolved through it, so a model cannot read or write outside
// the project it was pointed at — the cheapest meaningful safety boundary an
// agent can have.
type Workspace struct {
	root string
}

// NewWorkspace roots a workspace at dir, normalized to an absolute path.
func NewWorkspace(dir string) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve root: %w", err)
	}
	return &Workspace{root: filepath.Clean(abs)}, nil
}

// Root returns the absolute workspace root.
func (w *Workspace) Root() string { return w.root }

// Resolve maps a tool-supplied path (relative or absolute) to an absolute path
// guaranteed to live within the workspace, rejecting any attempt to escape it.
func (w *Workspace) Resolve(p string) (string, error) {
	var abs string
	if p == "" {
		abs = w.root
	} else if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Clean(filepath.Join(w.root, p))
	}

	rel, err := filepath.Rel(w.root, abs)
	if err != nil {
		return "", fmt.Errorf("path %q is not within the workspace", p)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the workspace root", p)
	}
	return abs, nil
}

// Rel renders an absolute path relative to the workspace root for display.
func (w *Workspace) Rel(abs string) string {
	if rel, err := filepath.Rel(w.root, abs); err == nil {
		return rel
	}
	return abs
}
