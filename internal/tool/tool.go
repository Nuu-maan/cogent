// Package tool defines the capability surface the model can act through. A Tool
// is anything with a name, a JSON-Schema description, and a Run method. The
// agent loop discovers tools via a Registry and never hard-codes any of them,
// so new capabilities are added by registration, not by editing the loop.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/Nuu-maan/cogent/internal/llm"
)

// Tool is a single capability exposed to the model.
type Tool interface {
	// Name is the stable identifier the model calls.
	Name() string
	// Description tells the model when and why to use the tool.
	Description() string
	// Schema is the JSON Schema (an object) describing Run's arguments.
	Schema() json.RawMessage
	// Run executes the tool. The returned string is fed back to the model as the
	// tool result. A non-nil error is surfaced to the model as an error result,
	// not crashed on — tools fail in-band.
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry is an ordered, concurrency-safe set of tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds t, panicking on a duplicate name — a wiring bug worth failing
// loudly at startup rather than silently shadowing.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tools[t.Name()]; dup {
		panic("tool: duplicate registration for " + t.Name())
	}
	r.tools[t.Name()] = t
}

// Get returns the tool registered under name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Specs returns the advertised form of every tool, sorted by name for a stable
// prompt (which keeps prompt caches warm across turns).
func (r *Registry) Specs() []llm.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]llm.ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		specs = append(specs, llm.ToolSpec{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}

// decodeArgs is a small helper for tools to unmarshal their arguments with a
// consistent, model-readable error.
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
