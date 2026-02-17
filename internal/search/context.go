package search

import (
	"bufio"
	"fmt"
	"os"
)

// ContextExtractor extracts surrounding lines from files for rich search results.
// Phase 2a: Provides N lines before/after matches to make results self-explanatory.
type ContextExtractor struct {
	linesBefore int
	linesAfter  int
}

// NewContextExtractor creates a context extractor with the specified line counts.
// Common values: 3 lines before/after for compact context, 5 for more detail.
func NewContextExtractor(before, after int) *ContextExtractor {
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	return &ContextExtractor{
		linesBefore: before,
		linesAfter:  after,
	}
}

// ExtractContext returns lines before and after a target line in a file.
// targetLine is 1-indexed. Returns empty slices if file can't be read.
// Handles edge cases: start of file (fewer before lines), end of file (fewer after lines).
func (e *ContextExtractor) ExtractContext(filePath string, targetLine int) (before []string, after []string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	// Use ring buffer for "before" lines to efficiently track last N lines
	beforeBuffer := make([]string, 0, e.linesBefore)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if lineNum < targetLine {
			// Collect lines before target (keep only last N)
			beforeBuffer = append(beforeBuffer, line)
			if len(beforeBuffer) > e.linesBefore {
				beforeBuffer = beforeBuffer[1:] // Shift left, drop oldest
			}
		} else if lineNum == targetLine {
			// Found target line, now collect lines after
			before = beforeBuffer

			// Collect lines after target
			for i := 0; i < e.linesAfter && scanner.Scan(); i++ {
				after = append(after, scanner.Text())
			}
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scanning file: %w", err)
	}

	// If we never reached targetLine, return empty slices
	if lineNum < targetLine {
		return []string{}, []string{}, nil
	}

	// Ensure non-nil slices
	if before == nil {
		before = []string{}
	}
	if after == nil {
		after = []string{}
	}

	return before, after, nil
}

// ExtractContextBatch extracts context for multiple target lines in the same file.
// More efficient than calling ExtractContext multiple times for the same file.
// targetLines must be sorted in ascending order.
func (e *ContextExtractor) ExtractContextBatch(filePath string, targetLines []int) (map[int]ContextLines, error) {
	if len(targetLines) == 0 {
		return map[int]ContextLines{}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	results := make(map[int]ContextLines)
	scanner := bufio.NewScanner(file)
	lineNum := 0
	targetIdx := 0 // Index into targetLines

	// Ring buffer for before lines
	beforeBuffer := make([]string, 0, e.linesBefore)

	for scanner.Scan() && targetIdx < len(targetLines) {
		lineNum++
		line := scanner.Text()
		targetLine := targetLines[targetIdx]

		if lineNum < targetLine {
			// Keep tracking before lines
			beforeBuffer = append(beforeBuffer, line)
			if len(beforeBuffer) > e.linesBefore {
				beforeBuffer = beforeBuffer[1:]
			}
		} else if lineNum == targetLine {
			// Found target, collect after lines
			afterLines := make([]string, 0, e.linesAfter)
			for i := 0; i < e.linesAfter && scanner.Scan(); i++ {
				afterLines = append(afterLines, scanner.Text())
				lineNum++
			}

			results[targetLine] = ContextLines{
				Before: append([]string{}, beforeBuffer...), // Copy buffer
				After:  afterLines,
			}

			targetIdx++

			// Reset buffer for next target
			beforeBuffer = make([]string, 0, e.linesBefore)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning file: %w", err)
	}

	return results, nil
}

// ContextLines represents context lines around a target.
type ContextLines struct {
	Before []string `json:"before"`
	After  []string `json:"after"`
}
