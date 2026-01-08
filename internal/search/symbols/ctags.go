package symbols

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// CtagsEntry represents a single entry from ctags JSON output
type CtagsEntry struct {
	Type     string `json:"_type"` // "tag" for symbol entries
	Name     string `json:"name"`
	Path     string `json:"path"`
	Pattern  string `json:"pattern"`
	Kind     string `json:"kind"`
	Line     int    `json:"line"`
	Language string `json:"language"`
	Scope    string `json:"scope"`
	ScopeKind string `json:"scopeKind"`
	Signature string `json:"signature"`
}

// CtagsAvailable checks if universal-ctags is installed and working
func CtagsAvailable() bool {
	cmd := exec.Command("ctags", "--version")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// Universal Ctags includes "Universal Ctags" in its version output
	return strings.Contains(string(output), "Universal Ctags")
}

// RunCtags runs universal-ctags on the given paths and returns parsed entries
// If paths is empty, runs on current directory recursively
func RunCtags(root string, paths []string) ([]CtagsEntry, error) {
	if !CtagsAvailable() {
		return nil, fmt.Errorf("universal-ctags not available")
	}

	args := []string{
		"--output-format=json",
		"--fields=+nKS",        // Include line number, kind, scope, signature
		"--kinds-all=*",        // Include all symbol kinds
		"--extras=+q",          // Include qualified tags
	}

	if len(paths) == 0 {
		// Recursive scan
		args = append(args, "-R")
		if root != "" && root != "." {
			args = append(args, root)
		} else {
			args = append(args, ".")
		}
	} else {
		args = append(args, paths...)
	}

	cmd := exec.Command("ctags", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ctags: %w", err)
	}

	var entries []CtagsEntry
	scanner := bufio.NewScanner(stdout)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry CtagsEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip malformed lines
			continue
		}

		// Only process tag entries (skip program info, etc.)
		if entry.Type != "tag" {
			continue
		}

		// Normalize path relative to root
		if root != "" && root != "." {
			if rel, err := filepath.Rel(root, entry.Path); err == nil {
				entry.Path = rel
			}
		}

		entries = append(entries, entry)
	}

	if err := cmd.Wait(); err != nil {
		// ctags may exit with error even if it produced output
		if len(entries) > 0 {
			return entries, nil
		}
		return nil, fmt.Errorf("ctags error: %w", err)
	}

	return entries, nil
}

// RunCtagsOnFile runs ctags on a single file
func RunCtagsOnFile(path string) ([]CtagsEntry, error) {
	return RunCtags("", []string{path})
}

// ToSymbol converts a CtagsEntry to a Symbol
func (e *CtagsEntry) ToSymbol() Symbol {
	scope := e.Scope
	if e.ScopeKind != "" && scope != "" {
		scope = e.ScopeKind + ":" + scope
	}

	return Symbol{
		Name:      e.Name,
		Kind:      normalizeKind(e.Kind),
		Path:      e.Path,
		Line:      e.Line,
		Language:  e.Language,
		Pattern:   e.Pattern,
		Scope:     scope,
	}
}

// normalizeKind normalizes ctags kind names to consistent values
func normalizeKind(kind string) string {
	// Map common ctags kinds to normalized names
	switch strings.ToLower(kind) {
	case "f", "func", "function", "method":
		return "function"
	case "c", "class":
		return "class"
	case "s", "struct":
		return "struct"
	case "i", "interface":
		return "interface"
	case "t", "type", "typedef":
		return "type"
	case "v", "var", "variable":
		return "variable"
	case "const", "constant":
		return "constant"
	case "p", "package":
		return "package"
	case "m", "member", "field":
		return "field"
	case "e", "enum", "enumerator":
		return "enum"
	default:
		return kind
	}
}
