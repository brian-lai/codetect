package tools

import (
	"fmt"

	"codetect/internal/search/files"
)

// getSnippetFnWithLimit returns a snippet reader with a configurable max length.
func getSnippetFnWithLimit(maxLen int) func(path string, start, end int) string {
	return func(path string, start, end int) string {
		result, err := files.GetFile(path, start, end)
		if err != nil {
			return fmt.Sprintf("[Error reading %s: %v]", path, err)
		}

		snippet := result.Content
		if maxLen > 0 && len(snippet) > maxLen {
			snippet = snippet[:maxLen] + "..."
		}

		return snippet
	}
}
