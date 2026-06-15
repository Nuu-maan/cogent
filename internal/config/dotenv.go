package config

import (
	"bufio"
	"os"
	"strings"
)

// loadDotenv reads KEY=VALUE pairs from a .env file and injects them into the
// process environment. It is intentionally tiny — no third-party dotenv
// dependency — and follows the conventional rules:
//
//   - A missing file is not an error.
//   - Blank lines and lines beginning with '#' are ignored.
//   - Surrounding single or double quotes are stripped from values.
//   - An existing environment variable is never overwritten (real env wins).
//   - Names containing characters invalid in an environment variable (such as
//     the hyphen in "API-KEY") are normalized by mapping them to underscores,
//     so "API-KEY" is exported as "API_KEY".
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = normalizeEnvName(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, unquote(strings.TrimSpace(val)))
	}
}

// normalizeEnvName replaces any character that is not a letter, digit, or
// underscore with an underscore, yielding a valid environment variable name.
func normalizeEnvName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
