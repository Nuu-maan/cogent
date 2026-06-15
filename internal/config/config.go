// Package config loads agent configuration from a JSON file and the
// environment, in that order of increasing precedence. It resolves the right API
// key for the chosen provider so secrets stay in the environment, never on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	Provider           string  `json:"provider"`
	Model              string  `json:"model"`
	BaseURL            string  `json:"base_url"`
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	HTTPTimeoutSeconds int     `json:"http_timeout_seconds"`
	Workspace          string  `json:"workspace"`
	SystemPromptFile   string  `json:"system_prompt_file"`
	Context            Context `json:"context"`

	// apiKey is resolved from the environment, never serialized.
	apiKey string
}

// Context configures window management.
type Context struct {
	MaxTokens    int    `json:"max_tokens"`
	KeepRecent   int    `json:"keep_recent"`
	SummaryModel string `json:"summary_model"`
}

// Default returns the baseline configuration before file or env overrides.
func Default() Config {
	return Config{
		Provider:           "anthropic",
		Model:              "claude-sonnet-4-6",
		Temperature:        0.0,
		MaxTokens:          4096,
		HTTPTimeoutSeconds: 0, // streaming: rely on context cancellation
		Workspace:          ".",
		Context: Context{
			MaxTokens:  24000,
			KeepRecent: 6,
		},
	}
}

// providerKeyEnv maps a provider to its conventional API-key environment
// variable. COGENT_API_KEY overrides all of them.
var providerKeyEnv = map[string]string{
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"ollama":     "", // local; no key required
}

// Load reads configuration from path (if it exists) layered over defaults, then
// applies environment overrides and resolves the API key. A missing file is not
// an error — defaults plus environment are a valid configuration.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("config: read %s: %w", path, err)
		}
	}

	cfg.applyEnv()
	cfg.resolveAPIKey()

	if cfg.Context.SummaryModel == "" {
		cfg.Context.SummaryModel = cfg.Model
	}
	return cfg, nil
}

// Validate checks that the configuration is usable before wiring up providers.
func (c Config) Validate() error {
	if c.Provider == "" {
		return errors.New("config: provider is required")
	}
	if c.Model == "" {
		return errors.New("config: model is required")
	}
	if _, ok := providerKeyEnv[c.Provider]; ok && c.Provider != "ollama" && c.apiKey == "" {
		return fmt.Errorf("config: missing API key for provider %q (set %s)", c.Provider, providerKeyEnv[c.Provider])
	}
	return nil
}

// APIKey returns the resolved provider credential.
func (c Config) APIKey() string { return c.apiKey }

func (c *Config) applyEnv() {
	if v := os.Getenv("COGENT_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := os.Getenv("COGENT_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("COGENT_BASE_URL"); v != "" {
		c.BaseURL = v
	}
	if v := os.Getenv("COGENT_WORKSPACE"); v != "" {
		c.Workspace = v
	}
	if v := os.Getenv("COGENT_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxTokens = n
		}
	}
}

func (c *Config) resolveAPIKey() {
	if v := os.Getenv("COGENT_API_KEY"); v != "" {
		c.apiKey = v
		return
	}
	if env := providerKeyEnv[c.Provider]; env != "" {
		c.apiKey = os.Getenv(env)
	}
}
