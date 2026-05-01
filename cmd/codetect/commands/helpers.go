package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// formatBytes converts bytes to human-readable format.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// isCodeFile returns true for files that should be embedded.
func isCodeFile(path string) bool {
	ext := filepath.Ext(path)
	codeExts := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".py": true, ".rb": true, ".java": true, ".c": true, ".cpp": true,
		".h": true, ".hpp": true, ".rs": true, ".swift": true, ".kt": true,
		".scala": true, ".php": true, ".cs": true, ".sh": true, ".sql": true,
	}
	return codeExts[ext]
}

// loadGitignore loads gitignore patterns from local .gitignore and global ~/.gitignore.
func loadGitignore(rootPath string) *ignore.GitIgnore {
	var patterns []string

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		if content, err := os.ReadFile(filepath.Join(homeDir, ".gitignore")); err == nil {
			for _, line := range splitLines(string(content)) {
				if line != "" && !isComment(line) {
					patterns = append(patterns, line)
				}
			}
		}
	}

	if content, err := os.ReadFile(filepath.Join(rootPath, ".gitignore")); err == nil {
		for _, line := range splitLines(string(content)) {
			if line != "" && !isComment(line) {
				patterns = append(patterns, line)
			}
		}
	}

	if len(patterns) == 0 {
		return nil
	}
	return ignore.CompileIgnoreLines(patterns...)
}

func splitLines(content string) []string {
	var lines []string
	start := 0
	for i, c := range content {
		if c == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func isComment(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "#")
}
