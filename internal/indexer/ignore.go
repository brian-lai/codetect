package indexer

import (
	"os"
	"path/filepath"

	ignore "github.com/sabhiram/go-gitignore"
)

// LoadCodetectIgnore loads .codetectignore patterns from the repository root.
// Returns nil if no .codetectignore file exists (no exclusions).
func LoadCodetectIgnore(repoRoot string) (*ignore.GitIgnore, error) {
	// Check for project .codetectignore
	projectIgnoreFile := filepath.Join(repoRoot, ".codetectignore")
	if _, err := os.Stat(projectIgnoreFile); err == nil {
		return ignore.CompileIgnoreFile(projectIgnoreFile)
	}

	// Check for global ~/.codetectignore
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalIgnoreFile := filepath.Join(homeDir, ".codetectignore")
		if _, err := os.Stat(globalIgnoreFile); err == nil {
			return ignore.CompileIgnoreFile(globalIgnoreFile)
		}
	}

	// No .codetectignore found, return nil (no exclusions)
	return nil, nil
}

// LoadCodetectIgnoreHierarchy loads .codetectignore patterns from both
// global (~/.codetectignore) and project (.codetectignore) locations,
// merging them together. Patterns from both files are combined with OR logic.
func LoadCodetectIgnoreHierarchy(repoRoot string) (*ignore.GitIgnore, error) {
	var patterns []string

	// 1. Load global ~/.codetectignore
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalFile := filepath.Join(homeDir, ".codetectignore")
		if content, err := os.ReadFile(globalFile); err == nil {
			globalPatterns := parseIgnoreLines(string(content))
			patterns = append(patterns, globalPatterns...)
		}
	}

	// 2. Load project .codetectignore
	projectFile := filepath.Join(repoRoot, ".codetectignore")
	if content, err := os.ReadFile(projectFile); err == nil {
		projectPatterns := parseIgnoreLines(string(content))
		patterns = append(patterns, projectPatterns...)
	}

	// If no patterns found, return nil (no exclusions)
	if len(patterns) == 0 {
		return nil, nil
	}

	// Compile all patterns together
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
