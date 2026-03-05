package config

import (
	"os"
	"testing"
)

func TestLoadPrivacyConfigFromEnv_Default(t *testing.T) {
	saved := os.Getenv("CODETECT_HASH_PATHS")
	os.Unsetenv("CODETECT_HASH_PATHS")
	defer func() {
		if saved != "" {
			os.Setenv("CODETECT_HASH_PATHS", saved)
		}
	}()

	cfg := LoadPrivacyConfigFromEnv()
	if cfg.HashPaths {
		t.Error("expected HashPaths=false by default")
	}
}

func TestLoadPrivacyConfigFromEnv_Enabled(t *testing.T) {
	saved := os.Getenv("CODETECT_HASH_PATHS")
	defer func() {
		if saved == "" {
			os.Unsetenv("CODETECT_HASH_PATHS")
		} else {
			os.Setenv("CODETECT_HASH_PATHS", saved)
		}
	}()

	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"enabled", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"off", false},
		{"disabled", false},
	}

	for _, tc := range tests {
		os.Setenv("CODETECT_HASH_PATHS", tc.value)
		cfg := LoadPrivacyConfigFromEnv()
		if cfg.HashPaths != tc.expected {
			t.Errorf("CODETECT_HASH_PATHS=%q: got HashPaths=%v, want %v", tc.value, cfg.HashPaths, tc.expected)
		}
	}
}
