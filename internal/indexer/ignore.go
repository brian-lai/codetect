package indexer

import (
	"log/slog"
	"os"
	"path/filepath"

	ignore "github.com/sabhiram/go-gitignore"
)

// defaultIgnoreExtensionPatterns are built-in gitignore patterns for non-code
// asset file extensions. These are prepended at lowest priority in
// LoadCodetectIgnoreHierarchy, so user patterns (including negation like
// !*.svg) naturally override them.
var defaultIgnoreExtensionPatterns = []string{
	// Images
	"*.svg", "*.png", "*.jpg", "*.jpeg", "*.gif", "*.ico", "*.bmp", "*.webp",
	// Fonts
	"*.woff", "*.woff2", "*.ttf", "*.eot", "*.otf",
	// Media
	"*.mp3", "*.mp4", "*.wav", "*.avi", "*.mov",
	// Archives
	"*.zip", "*.tar", "*.gz", "*.tgz", "*.bz2", "*.rar", "*.7z",
	// Other non-code
	"*.pdf", "*.map", "*.min.js", "*.min.css",
}

// xdgCodetectConfigDir returns the XDG-based codetect config directory,
// consistent with the config directory used by registry.go.
func xdgCodetectConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "codetect")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "codetect")
}

// LoadCodetectIgnore loads .codetectignore patterns using priority-based lookup.
// Returns the first ignore file found, checking in order:
//  1. <repoRoot>/.codetectignore (project-specific, highest priority)
//  2. ~/.config/codetect/ignore (XDG global)
//  3. ~/.codetectignore (legacy global, loads with deprecation warning)
func LoadCodetectIgnore(repoRoot string) (*ignore.GitIgnore, error) {
	// 1. Check for project .codetectignore
	projectIgnoreFile := filepath.Join(repoRoot, ".codetectignore")
	if _, err := os.Stat(projectIgnoreFile); err == nil {
		return ignore.CompileIgnoreFile(projectIgnoreFile)
	}

	xdgIgnoreFile := filepath.Join(xdgCodetectConfigDir(), "ignore")

	// 2. Check for XDG global ~/.config/codetect/ignore
	if _, err := os.Stat(xdgIgnoreFile); err == nil {
		return ignore.CompileIgnoreFile(xdgIgnoreFile)
	}

	// 3. Check for legacy ~/.codetectignore
	homeDir, err := os.UserHomeDir()
	if err == nil {
		legacyIgnoreFile := filepath.Join(homeDir, ".codetectignore")
		if _, err := os.Stat(legacyIgnoreFile); err == nil {
			slog.Warn("~/.codetectignore is deprecated; move it to the XDG config dir",
				"legacy_path", legacyIgnoreFile,
				"new_path", xdgIgnoreFile)
			return ignore.CompileIgnoreFile(legacyIgnoreFile)
		}
	}

	return nil, nil
}

// LoadCodetectIgnoreHierarchy loads and merges .codetectignore patterns from
// all three locations. Patterns are combined with OR logic (a file is excluded
// if it matches any pattern from any source).
//
// Load order (project patterns appended last for negation precedence):
//  1. ~/.config/codetect/ignore — XDG global
//  2. ~/.codetectignore — legacy global (loads with deprecation warning)
//  3. <repoRoot>/.codetectignore — project-specific (highest priority)
func LoadCodetectIgnoreHierarchy(repoRoot string) (*ignore.GitIgnore, error) {
	// Start with default extension patterns at lowest priority (position 0).
	// User patterns appended later override these via gitignore last-match-wins semantics.
	patterns := make([]string, len(defaultIgnoreExtensionPatterns))
	copy(patterns, defaultIgnoreExtensionPatterns)

	xdgIgnoreFile := filepath.Join(xdgCodetectConfigDir(), "ignore")

	// 1. Load XDG global ~/.config/codetect/ignore
	if content, err := os.ReadFile(xdgIgnoreFile); err == nil {
		patterns = append(patterns, parseIgnoreLines(string(content))...)
	}

	// 2. Load legacy ~/.codetectignore (with deprecation warning)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		legacyFile := filepath.Join(homeDir, ".codetectignore")
		if content, err := os.ReadFile(legacyFile); err == nil {
			slog.Warn("~/.codetectignore is deprecated; move it to the XDG config dir",
				"legacy_path", legacyFile,
				"new_path", xdgIgnoreFile)
			patterns = append(patterns, parseIgnoreLines(string(content))...)
		}
	}

	// 3. Load project .codetectignore (highest priority)
	projectFile := filepath.Join(repoRoot, ".codetectignore")
	if content, err := os.ReadFile(projectFile); err == nil {
		patterns = append(patterns, parseIgnoreLines(string(content))...)
	}

	return ignore.CompileIgnoreLines(patterns...), nil
}

// parseIgnoreLines parses .codetectignore content into individual pattern lines.
// Filters out empty lines and comments.
func parseIgnoreLines(content string) []string {
	var patterns []string
	lines := splitLines(content)

	for _, line := range lines {
		// Trim whitespace
		line = trimSpace(line)

		// Skip empty lines and comments
		if line == "" || startsWithHash(line) {
			continue
		}

		patterns = append(patterns, line)
	}

	return patterns
}

// splitLines splits content into lines (handles both \n and \r\n)
func splitLines(content string) []string {
	var lines []string
	var line string

	for _, char := range content {
		if char == '\n' {
			lines = append(lines, line)
			line = ""
		} else if char != '\r' {
			line += string(char)
		}
	}

	// Add last line if not empty
	if line != "" {
		lines = append(lines, line)
	}

	return lines
}

// trimSpace removes leading/trailing whitespace from a string
func trimSpace(s string) string {
	// Trim leading whitespace
	for len(s) > 0 && isWhitespace(s[0]) {
		s = s[1:]
	}

	// Trim trailing whitespace
	for len(s) > 0 && isWhitespace(s[len(s)-1]) {
		s = s[:len(s)-1]
	}

	return s
}

// isWhitespace checks if a byte is whitespace
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t'
}

// startsWithHash checks if a line starts with #
func startsWithHash(s string) bool {
	return len(s) > 0 && s[0] == '#'
}
