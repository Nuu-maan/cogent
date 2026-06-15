package llm

import (
	"fmt"
	"sort"
	"sync"
)

// ProviderConfig is the wiring a [Factory] needs to construct a [Provider].
// It is intentionally provider-neutral; backend-specific knobs belong in the
// backend's own package, defaulted from these fields.
type ProviderConfig struct {
	// APIKey authenticates with the backend. May be empty for local servers
	// such as Ollama.
	APIKey string
	// BaseURL overrides the backend's default endpoint. Empty means default.
	BaseURL string
	// HTTPTimeoutSeconds bounds a single request; zero means the backend default.
	HTTPTimeoutSeconds int
}

// Factory constructs a [Provider] from config. Backends register one of these.
type Factory func(ProviderConfig) (Provider, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register makes a backend available under name. It is meant to be called from a
// backend package's init function, mirroring database/sql driver registration.
// Registering a duplicate or nil factory panics, since both are programmer bugs.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		panic("llm: Register called with nil factory for " + name)
	}
	if _, dup := factories[name]; dup {
		panic("llm: Register called twice for provider " + name)
	}
	factories[name] = f
}

// Open constructs the provider registered under name.
func Open(name string, cfg ProviderConfig) (Provider, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q (registered: %v)", name, Registered())
	}
	return f(cfg)
}

// Registered returns the sorted names of all registered providers.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
