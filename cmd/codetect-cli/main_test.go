package main

import (
	"bytes"
	"strings"
	"testing"

	"codetect/internal/mcp"
	"codetect/internal/tools"
)

func TestRun_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected usage message on stderr")
	}
}

func TestRun_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if stdout.Len() == 0 {
		t.Error("expected usage message on stdout")
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected error message on stderr")
	}
}

func TestRun_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if stdout.Len() == 0 {
		t.Error("expected version on stdout")
	}
}

// --- Helper to create a server with all tools registered ---

func newTestServer() *mcp.Server {
	server := mcp.NewServer("test", "1.0")
	config := tools.DefaultConfig()
	tools.RegisterAll(server, config)
	return server
}

// --- search subcommand tests ---

func TestBuildSearchArgs_Defaults(t *testing.T) {
	args, err := buildSearchArgs([]string{"myquery"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["query"] != "myquery" {
		t.Errorf("expected query 'myquery', got %v", args["query"])
	}
	if _, ok := args["top_k"]; ok {
		t.Error("expected no top_k when not specified")
	}
	if _, ok := args["detail"]; ok {
		t.Error("expected no detail when not specified")
	}
}

func TestBuildSearchArgs_AllFlags(t *testing.T) {
	args, err := buildSearchArgs([]string{"--top-k", "5", "--detail", "rich", "myquery"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["query"] != "myquery" {
		t.Errorf("expected query 'myquery', got %v", args["query"])
	}
	if args["top_k"] != float64(5) {
		t.Errorf("expected top_k 5.0, got %v", args["top_k"])
	}
	if args["detail"] != "rich" {
		t.Errorf("expected detail 'rich', got %v", args["detail"])
	}
}

func TestBuildSearchArgs_MissingQuery(t *testing.T) {
	_, err := buildSearchArgs([]string{})
	if err == nil {
		t.Error("expected error for missing query")
	}
}

func TestRunSearch_CallsCorrectTool(t *testing.T) {
	server := newTestServer()
	var stdout, stderr bytes.Buffer
	// search for a pattern that exists in this repo
	code := runSearchWithServer([]string{"func main"}, server, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected JSON output on stdout")
	}
	// Output should be valid JSON with results
	if !strings.Contains(stdout.String(), "results") {
		t.Errorf("expected output to contain 'results', got: %s", stdout.String())
	}
}

// --- file subcommand tests ---

func TestBuildFileArgs_PathOnly(t *testing.T) {
	args, err := buildFileArgs([]string{"src/main.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["path"] != "src/main.go" {
		t.Errorf("expected path 'src/main.go', got %v", args["path"])
	}
	if _, ok := args["start_line"]; ok {
		t.Error("expected no start_line when not specified")
	}
	if _, ok := args["end_line"]; ok {
		t.Error("expected no end_line when not specified")
	}
}

func TestBuildFileArgs_WithLineRange(t *testing.T) {
	args, err := buildFileArgs([]string{"--start-line", "10", "--end-line", "20", "src/main.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["path"] != "src/main.go" {
		t.Errorf("expected path 'src/main.go', got %v", args["path"])
	}
	if args["start_line"] != float64(10) {
		t.Errorf("expected start_line 10.0, got %v", args["start_line"])
	}
	if args["end_line"] != float64(20) {
		t.Errorf("expected end_line 20.0, got %v", args["end_line"])
	}
}

func TestBuildFileArgs_MissingPath(t *testing.T) {
	_, err := buildFileArgs([]string{})
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestRunFile_CallsCorrectTool(t *testing.T) {
	server := newTestServer()
	var stdout, stderr bytes.Buffer
	// Use the test file itself (guaranteed to exist in the test's working directory)
	code := runFileWithServer([]string{"main.go"}, server, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected JSON output on stdout")
	}
	if !strings.Contains(stdout.String(), "content") {
		t.Errorf("expected output to contain 'content', got: %s", stdout.String())
	}
}

// --- symbols subcommand tests ---

func TestBuildSymbolsArgs_Find(t *testing.T) {
	args, err := buildSymbolsArgs([]string{"find", "MyFunc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["mode"] != "find" {
		t.Errorf("expected mode 'find', got %v", args["mode"])
	}
	if args["name"] != "MyFunc" {
		t.Errorf("expected name 'MyFunc', got %v", args["name"])
	}
}

func TestBuildSymbolsArgs_FindWithFlags(t *testing.T) {
	args, err := buildSymbolsArgs([]string{"find", "--kind", "function", "--limit", "5", "MyFunc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["mode"] != "find" {
		t.Errorf("expected mode 'find', got %v", args["mode"])
	}
	if args["name"] != "MyFunc" {
		t.Errorf("expected name 'MyFunc', got %v", args["name"])
	}
	if args["kind"] != "function" {
		t.Errorf("expected kind 'function', got %v", args["kind"])
	}
	if args["limit"] != float64(5) {
		t.Errorf("expected limit 5.0, got %v", args["limit"])
	}
}

func TestBuildSymbolsArgs_List(t *testing.T) {
	args, err := buildSymbolsArgs([]string{"list", "src/main.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["mode"] != "list" {
		t.Errorf("expected mode 'list', got %v", args["mode"])
	}
	if args["path"] != "src/main.go" {
		t.Errorf("expected path 'src/main.go', got %v", args["path"])
	}
}

func TestBuildSymbolsArgs_MissingSubcommand(t *testing.T) {
	_, err := buildSymbolsArgs([]string{})
	if err == nil {
		t.Error("expected error for missing subcommand")
	}
}

func TestBuildSymbolsArgs_FindMissingName(t *testing.T) {
	_, err := buildSymbolsArgs([]string{"find"})
	if err == nil {
		t.Error("expected error for find without name")
	}
}

func TestBuildSymbolsArgs_ListMissingPath(t *testing.T) {
	_, err := buildSymbolsArgs([]string{"list"})
	if err == nil {
		t.Error("expected error for list without path")
	}
}

func TestRunSymbols_CallsCorrectTool(t *testing.T) {
	server := newTestServer()
	var stdout, stderr bytes.Buffer
	// This will fail gracefully (no symbol index) but should still invoke the tool
	code := runSymbolsWithServer([]string{"find", "NewServer"}, server, &stdout, &stderr)
	// Expect non-zero since there's no real symbol index in test context
	// But we verify the tool was invoked (either error or result output)
	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Errorf("expected some output, got none; code=%d", code)
	}
}

// --- hybrid subcommand tests ---

func TestBuildHybridArgs_Defaults(t *testing.T) {
	args, err := buildHybridArgs([]string{"myquery"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["query"] != "myquery" {
		t.Errorf("expected query 'myquery', got %v", args["query"])
	}
	if _, ok := args["limit"]; ok {
		t.Error("expected no limit when not specified")
	}
	if _, ok := args["rerank"]; ok {
		t.Error("expected no rerank when not specified")
	}
}

func TestBuildHybridArgs_AllFlags(t *testing.T) {
	args, err := buildHybridArgs([]string{"--limit", "20", "--rerank", "--detail", "rich", "myquery"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["query"] != "myquery" {
		t.Errorf("expected query 'myquery', got %v", args["query"])
	}
	if args["limit"] != float64(20) {
		t.Errorf("expected limit 20.0, got %v", args["limit"])
	}
	if args["rerank"] != true {
		t.Errorf("expected rerank true, got %v", args["rerank"])
	}
	if args["detail"] != "rich" {
		t.Errorf("expected detail 'rich', got %v", args["detail"])
	}
}

func TestBuildHybridArgs_MissingQuery(t *testing.T) {
	_, err := buildHybridArgs([]string{})
	if err == nil {
		t.Error("expected error for missing query")
	}
}

func TestRunHybrid_CallsCorrectTool(t *testing.T) {
	server := newTestServer()
	var stdout, stderr bytes.Buffer
	code := runHybridWithServer([]string{"func main"}, server, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected JSON output on stdout")
	}
	if !strings.Contains(stdout.String(), "results") {
		t.Errorf("expected output to contain 'results', got: %s", stdout.String())
	}
}
