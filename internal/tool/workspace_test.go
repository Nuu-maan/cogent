package tool

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceResolve(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"relative file", "src/main.go", false},
		{"nested relative", "a/b/c.txt", false},
		{"empty resolves to root", "", false},
		{"dot resolves to root", ".", false},
		{"parent escape", "../secret", true},
		{"deep parent escape", "a/../../etc/passwd", true},
		{"absolute outside", filepath.Join(filepath.Dir(root), "elsewhere"), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ws.Resolve(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Resolve(%q) = %q, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q) unexpected error: %v", tc.input, err)
			}
			if rel, _ := filepath.Rel(root, got); rel == ".." || filepath.IsAbs(rel) && rel != got {
				t.Fatalf("Resolve(%q) = %q, escapes root %q", tc.input, got, root)
			}
		})
	}
}
