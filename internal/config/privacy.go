package config

import "os"

// PrivacyConfig holds privacy-related configuration.
type PrivacyConfig struct {
	// HashPaths enables SHA-256 hashing of file paths at rest in the database.
	// When true, repo_root and path columns in chunk_locations (and failed_chunks)
	// are stored as SHA-256 hashes. A local sidecar file maintains the reverse
	// mapping so search results still return real paths.
	HashPaths bool
}

// LoadPrivacyConfigFromEnv loads privacy configuration from environment variables.
//
//   - CODETECT_HASH_PATHS: Enable path hashing at rest (default: false)
func LoadPrivacyConfigFromEnv() PrivacyConfig {
	cfg := PrivacyConfig{}

	if v := os.Getenv("CODETECT_HASH_PATHS"); v != "" {
		cfg.HashPaths = parseBool(v, false)
	}

	return cfg
}
