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
