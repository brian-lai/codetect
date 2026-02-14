package tools

import (
	"encoding/json"
	"testing"

	"codetect/internal/fusion"
	"codetect/internal/search/keyword"
)

// --- DetailLevel parsing ---

func TestParseDetailLevel_Default(t *testing.T) {
	args := map[string]any{"query": "foo"}
	if got := ParseDetailLevel(args); got != DetailStandard {
		t.Errorf("expected DetailStandard, got %v", got)
	}
}

func TestParseDetailLevel_Explicit(t *testing.T) {
	tests := []struct {
		input string
		want  DetailLevel
	}{
		{"minimal", DetailMinimal},
		{"standard", DetailStandard},
		{"rich", DetailRich},
		{"INVALID", DetailStandard}, // fallback
		{"", DetailStandard},        // fallback
	}
	for _, tt := range tests {
		args := map[string]any{"detail": tt.input}
		if got := ParseDetailLevel(args); got != tt.want {
			t.Errorf("ParseDetailLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDetailLevel_ShouldEnrich(t *testing.T) {
	if DetailMinimal.ShouldEnrich() {
		t.Error("minimal should not enrich")
	}
	if DetailStandard.ShouldEnrich() {
		t.Error("standard should not enrich")
	}
	if !DetailRich.ShouldEnrich() {
		t.Error("rich should enrich")
	}
}

func TestDetailLevel_ShouldIncludeSnippets(t *testing.T) {
	if DetailMinimal.ShouldIncludeSnippets() {
		t.Error("minimal should not include snippets")
	}
	if !DetailStandard.ShouldIncludeSnippets() {
		t.Error("standard should include snippets")
	}
	if !DetailRich.ShouldIncludeSnippets() {
		t.Error("rich should include snippets")
	}
}

// --- Snippet budgeting ---

func TestSnippetMaxLen(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{1, 500},
		{5, 500},
		{6, 300},
		{10, 300},
		{11, 150},
		{20, 150},
	}
	for _, tt := range tests {
		if got := SnippetMaxLen(tt.count); got != tt.want {
			t.Errorf("SnippetMaxLen(%d) = %d, want %d", tt.count, got, tt.want)
		}
	}
}

// --- RRF result marshaling by detail level ---

func TestMarshalRRFByDetail_Minimal(t *testing.T) {
	results := []fusion.RRFResult{
		{Result: fusion.Result{Path: "a.go", Line: 10, Snippet: "code here"}},
		{Result: fusion.Result{Path: "b.go", Line: 20, Snippet: "more code"}},
	}

	data, err := MarshalRRFByDetail(results, DetailMinimal)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(parsed.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(parsed.Results))
	}

	r := parsed.Results[0]
	if r["path"] != "a.go" {
		t.Errorf("expected path 'a.go', got %v", r["path"])
	}
	if _, has := r["snippet"]; has {
		t.Error("minimal should not include snippet")
	}
	if _, has := r["parent_scope"]; has {
		t.Error("minimal should not include parent_scope")
	}
}

func TestMarshalRRFByDetail_Standard(t *testing.T) {
	results := []fusion.RRFResult{
		{Result: fusion.Result{
			Path:        "a.go",
			Line:        10,
			Snippet:     "func main() {}",
			ParentScope: "main", // should be excluded at standard level
		}},
	}

	data, err := MarshalRRFByDetail(results, DetailStandard)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	r := parsed.Results[0]
	if r["path"] != "a.go" {
		t.Errorf("expected path 'a.go', got %v", r["path"])
	}
	if r["snippet"] != "func main() {}" {
		t.Errorf("standard should include snippet")
	}
	if _, has := r["parent_scope"]; has {
		t.Error("standard should not include parent_scope")
	}
}

func TestMarshalRRFByDetail_Rich(t *testing.T) {
	results := []fusion.RRFResult{
		{Result: fusion.Result{
			Path:          "a.go",
			Line:          10,
			Snippet:       "func main() {}",
			ParentScope:   "main",
			ScopeKind:     "function",
			ContextBefore: []string{"package main", ""},
			ContextAfter:  []string{"}", ""},
		}},
	}

	data, err := MarshalRRFByDetail(results, DetailRich)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	r := parsed.Results[0]
	if r["parent_scope"] != "main" {
		t.Error("rich should include parent_scope")
	}
	if r["scope_kind"] != "function" {
		t.Error("rich should include scope_kind")
	}
}

// --- Keyword result marshaling by detail level ---

func TestMarshalKeywordByDetail_Minimal(t *testing.T) {
	results := []keyword.Result{
		{Path: "a.go", LineStart: 10, Snippet: "code here"},
	}

	data, err := MarshalKeywordByDetail(results, DetailMinimal)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	r := parsed.Results[0]
	if _, has := r["snippet"]; has {
		t.Error("minimal should not include snippet")
	}
}

func TestMarshalKeywordByDetail_Standard_TruncatesSnippets(t *testing.T) {
	longSnippet := make([]byte, 600)
	for i := range longSnippet {
		longSnippet[i] = 'x'
	}

	results := make([]keyword.Result, 8) // 6-10 range → maxLen 300
	for i := range results {
		results[i] = keyword.Result{Path: "a.go", LineStart: i + 1, Snippet: string(longSnippet)}
	}

	data, err := MarshalKeywordByDetail(results, DetailStandard)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed struct {
		Results []struct {
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// 8 results → SnippetMaxLen = 300, plus "..." = 303
	for i, r := range parsed.Results {
		if len(r.Snippet) > 303 {
			t.Errorf("result %d snippet length %d exceeds budget 303", i, len(r.Snippet))
		}
	}
}
