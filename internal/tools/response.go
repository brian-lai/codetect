package tools

import (
	"encoding/json"

	"codetect/internal/fusion"
	"codetect/internal/search/keyword"
)

// DetailLevel controls response verbosity.
type DetailLevel int

const (
	DetailMinimal  DetailLevel = iota // path + line only
	DetailStandard                    // path + line + snippet (default)
	DetailRich                        // path + line + snippet + enrichment context
)

// ParseDetailLevel extracts the detail level from tool args.
// Defaults to DetailStandard if not specified or invalid.
func ParseDetailLevel(args map[string]any) DetailLevel {
	s, ok := args["detail"].(string)
	if !ok {
		return DetailStandard
	}
	switch s {
	case "minimal":
		return DetailMinimal
	case "standard":
		return DetailStandard
	case "rich":
		return DetailRich
	default:
		return DetailStandard
	}
}

// ShouldEnrich returns true if this detail level includes enrichment context.
func (d DetailLevel) ShouldEnrich() bool {
	return d == DetailRich
}

// ShouldIncludeSnippets returns true if this detail level includes snippets.
func (d DetailLevel) ShouldIncludeSnippets() bool {
	return d >= DetailStandard
}

// SnippetMaxLen returns the maximum snippet length based on result count.
// Fewer results get longer snippets; more results get shorter ones.
func SnippetMaxLen(count int) int {
	switch {
	case count <= 5:
		return 500
	case count <= 10:
		return 300
	default:
		return 150
	}
}

// truncateSnippet trims a snippet to maxLen, appending "..." if truncated.
func truncateSnippet(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- Minimal result types (path + line only) ---

type minimalResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line,omitempty"`
}

type minimalKeywordResult struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end,omitempty"`
}

// --- Standard result types (path + line + snippet) ---

type standardResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type standardKeywordResult struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
}

// --- Rich result types (full enrichment) ---

type richResult struct {
	Path          string   `json:"path"`
	Line          int      `json:"line"`
	EndLine       int      `json:"end_line,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	ParentScope   string   `json:"parent_scope,omitempty"`
	ScopeKind     string   `json:"scope_kind,omitempty"`
	ReceiverType  string   `json:"receiver_type,omitempty"`
	ContextBefore []string `json:"context_before,omitempty"`
	ContextAfter  []string `json:"context_after,omitempty"`
}

type richKeywordResult struct {
	Path          string   `json:"path"`
	LineStart     int      `json:"line_start"`
	LineEnd       int      `json:"line_end,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	ParentScope   string   `json:"parent_scope,omitempty"`
	ScopeKind     string   `json:"scope_kind,omitempty"`
	ReceiverType  string   `json:"receiver_type,omitempty"`
	ContextBefore []string `json:"context_before,omitempty"`
	ContextAfter  []string `json:"context_after,omitempty"`
}

// --- Wrapper for JSON output ---

type resultWrapper struct {
	Results interface{} `json:"results"`
}

// MarshalRRFByDetail marshals RRF results at the given detail level.
func MarshalRRFByDetail(results []fusion.RRFResult, detail DetailLevel) ([]byte, error) {
	maxLen := SnippetMaxLen(len(results))

	switch detail {
	case DetailMinimal:
		out := make([]minimalResult, len(results))
		for i, r := range results {
			out[i] = minimalResult{
				Path:    r.Path,
				Line:    r.Line,
				EndLine: r.EndLine,
			}
		}
		return json.Marshal(resultWrapper{Results: out})

	case DetailRich:
		out := make([]richResult, len(results))
		for i, r := range results {
			out[i] = richResult{
				Path:          r.Path,
				Line:          r.Line,
				EndLine:       r.EndLine,
				Snippet:       truncateSnippet(r.Snippet, maxLen),
				ParentScope:   r.ParentScope,
				ScopeKind:     r.ScopeKind,
				ReceiverType:  r.ReceiverType,
				ContextBefore: r.ContextBefore,
				ContextAfter:  r.ContextAfter,
			}
		}
		return json.Marshal(resultWrapper{Results: out})

	default: // DetailStandard
		out := make([]standardResult, len(results))
		for i, r := range results {
			out[i] = standardResult{
				Path:    r.Path,
				Line:    r.Line,
				EndLine: r.EndLine,
				Snippet: truncateSnippet(r.Snippet, maxLen),
			}
		}
		return json.Marshal(resultWrapper{Results: out})
	}
}

// MarshalKeywordByDetail marshals keyword results at the given detail level.
func MarshalKeywordByDetail(results []keyword.Result, detail DetailLevel) ([]byte, error) {
	maxLen := SnippetMaxLen(len(results))

	switch detail {
	case DetailMinimal:
		out := make([]minimalKeywordResult, len(results))
		for i, r := range results {
			out[i] = minimalKeywordResult{
				Path:      r.Path,
				LineStart: r.LineStart,
				LineEnd:   r.LineEnd,
			}
		}
		return json.Marshal(resultWrapper{Results: out})

	case DetailRich:
		out := make([]richKeywordResult, len(results))
		for i, r := range results {
			out[i] = richKeywordResult{
				Path:          r.Path,
				LineStart:     r.LineStart,
				LineEnd:       r.LineEnd,
				Snippet:       truncateSnippet(r.Snippet, maxLen),
				ParentScope:   r.ParentScope,
				ScopeKind:     r.ScopeKind,
				ReceiverType:  r.ReceiverType,
				ContextBefore: r.ContextBefore,
				ContextAfter:  r.ContextAfter,
			}
		}
		return json.Marshal(resultWrapper{Results: out})

	default: // DetailStandard
		out := make([]standardKeywordResult, len(results))
		for i, r := range results {
			out[i] = standardKeywordResult{
				Path:      r.Path,
				LineStart: r.LineStart,
				LineEnd:   r.LineEnd,
				Snippet:   truncateSnippet(r.Snippet, maxLen),
			}
		}
		return json.Marshal(resultWrapper{Results: out})
	}
}
