package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxReadBytes caps a single read so a stray huge file can't blow the context
// window or memory.
const maxReadBytes = 256 << 10 // 256 KiB

// ReadFile returns the contents of a file within the workspace.
type ReadFile struct{ WS *Workspace }

func (t *ReadFile) Name() string { return "read_file" }
func (t *ReadFile) Description() string {
	return "Read a UTF-8 text file from the workspace and return its contents."
}
func (t *ReadFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path relative to the workspace root."}
		},
		"required": ["path"]
	}`)
}

func (t *ReadFile) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return "", err
	}
	abs, err := t.WS.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a file", args.Path)
	}
	if info.Size() > maxReadBytes {
		return "", fmt.Errorf("%s is %d bytes; exceeds the %d-byte read limit", args.Path, info.Size(), maxReadBytes)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteFile creates or overwrites a file within the workspace.
type WriteFile struct{ WS *Workspace }

func (t *WriteFile) Name() string { return "write_file" }
func (t *WriteFile) Description() string {
	return "Create or overwrite a file in the workspace with the given content."
}
func (t *WriteFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path relative to the workspace root."},
			"content": {"type": "string", "description": "Full file content to write."}
		},
		"required": ["path", "content"]
	}`)
}

func (t *WriteFile) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return "", err
	}
	abs, err := t.WS.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}

// EditFile performs an exact string replacement in an existing file.
type EditFile struct{ WS *Workspace }

func (t *EditFile) Name() string { return "edit_file" }
func (t *EditFile) Description() string {
	return "Replace an exact string in a file. By default old_string must appear exactly once; set replace_all to replace every occurrence."
}
func (t *EditFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path relative to the workspace root."},
			"old_string": {"type": "string", "description": "Exact text to find."},
			"new_string": {"type": "string", "description": "Replacement text."},
			"replace_all": {"type": "boolean", "description": "Replace all occurrences instead of requiring a unique match."}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *EditFile) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return "", err
	}
	if args.OldString == args.NewString {
		return "", fmt.Errorf("old_string and new_string are identical")
	}
	abs, err := t.WS.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	content := string(b)
	n := strings.Count(content, args.OldString)
	switch {
	case n == 0:
		return "", fmt.Errorf("old_string not found in %s", args.Path)
	case n > 1 && !args.ReplaceAll:
		return "", fmt.Errorf("old_string appears %d times in %s; make it unique or set replace_all", n, args.Path)
	}
	updated := strings.ReplaceAll(content, args.OldString, args.NewString)
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced %d occurrence(s) in %s", n, args.Path), nil
}

// ListDir lists the immediate entries of a directory within the workspace.
type ListDir struct{ WS *Workspace }

func (t *ListDir) Name() string { return "list_dir" }
func (t *ListDir) Description() string {
	return "List the files and subdirectories directly inside a workspace directory."
}
func (t *ListDir) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path relative to the workspace root. Defaults to the root."}
		}
	}`)
}

func (t *ListDir) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return "", err
	}
	abs, err := t.WS.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name()+"/")
		} else {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(names, "\n"), nil
}
