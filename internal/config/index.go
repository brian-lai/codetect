package config

import (
	"os"
	"strings"
)

// IndexBackend specifies which symbol indexing backend to use
type IndexBackend string

const (
	// IndexBackendAuto uses ast-grep (default)
	IndexBackendAuto IndexBackend = "auto"

	// IndexBackendAstGrep uses ast-grep only (errors on unsupported languages)
	IndexBackendAstGrep IndexBackend = "ast-grep"
)

// IndexConfig holds configuration for symbol indexing
type IndexConfig struct {
	// Backend specifies which indexing tool to use
	Backend IndexBackend
}

// LoadIndexConfigFromEnv loads indexing configuration from environment variables.
// Supports the following variable:
//   - CODETECT_INDEX_BACKEND: Backend to use ("auto" or "ast-grep")
//
// If no environment variable is set, defaults to "auto".
func LoadIndexConfigFromEnv() IndexConfig {
	cfg := IndexConfig{
		Backend: IndexBackendAuto,
	}

	if backend := os.Getenv("CODETECT_INDEX_BACKEND"); backend != "" {
		switch strings.ToLower(backend) {
		case "auto", "hybrid":
			cfg.Backend = IndexBackendAuto
		case "ast-grep", "astgrep", "sg":
			cfg.Backend = IndexBackendAstGrep
		default:
			// Unknown backend, use default
			cfg.Backend = IndexBackendAuto
		}
	}

	return cfg
}

// UseAstGrep returns true if ast-grep should be used for indexing
func (c IndexConfig) UseAstGrep() bool {
	return c.Backend == IndexBackendAuto || c.Backend == IndexBackendAstGrep
}

// RequireAstGrep returns true if ast-grep is required (not optional)
func (c IndexConfig) RequireAstGrep() bool {
	return c.Backend == IndexBackendAstGrep
}

// String returns a human-readable description of the index configuration
func (c IndexConfig) String() string {
	switch c.Backend {
	case IndexBackendAstGrep:
		return "ast-grep only"
	default:
		return "ast-grep (auto)"
	}
}
