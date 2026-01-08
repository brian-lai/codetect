package files

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileResult is the output of a file read operation
type FileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// GetFile reads a file with optional line range slicing
// startLine and endLine are 1-indexed, inclusive
// If startLine is 0, reads from beginning
// If endLine is 0, reads to end
func GetFile(path string, startLine, endLine int) (*FileResult, error) {
	// Resolve to absolute path and validate
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Check file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("accessing file: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", path)
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	// Read with line slicing
	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0

	// Increase buffer for large lines
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		lineNum++

		// Skip lines before startLine
		if startLine > 0 && lineNum < startLine {
			continue
		}

		// Stop after endLine
		if endLine > 0 && lineNum > endLine {
			break
		}

		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Handle case where startLine is beyond file end
	if startLine > 0 && lineNum < startLine {
		return nil, fmt.Errorf("start_line %d is beyond end of file (%d lines)", startLine, lineNum)
	}

	return &FileResult{
		Path:    path,
		Content: strings.Join(lines, "\n"),
	}, nil
}

// GetFileLines returns specific lines from a file (1-indexed, inclusive)
func GetFileLines(path string, start, end int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum >= start && lineNum <= end {
			lines = append(lines, scanner.Text())
		}
		if end > 0 && lineNum > end {
			break
		}
	}

	return lines, scanner.Err()
}
