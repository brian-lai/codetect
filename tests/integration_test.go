package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegrationSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for ripgrep dependency
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not available, skipping integration test")
	}

	// Create temporary directory with sample files
	tmpDir := t.TempDir()

	// Create sample Go files with known symbols
	files := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	result := calculate(5, 3)
	fmt.Println(result)
}

func calculate(a, b int) int {
	return a + b
}
`,
		"server.go": `package main

type Server struct {
	Port int
	Host string
}

func NewServer(port int) *Server {
	return &Server{
		Port: port,
		Host: "localhost",
	}
}

func (s *Server) Start() error {
	return nil
}
`,
		"utils.go": `package main

const MaxRetries = 3

var GlobalConfig = "default"

func Helper(input string) string {
	return input + "_processed"
}
`,
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file %s: %v", name, err)
		}
	}

	// Step 1: Build the indexer binary
	// Get the repository root (parent of tests/ directory)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot := filepath.Dir(cwd) // Go up from tests/ to repo root

	indexerBin := filepath.Join(tmpDir, "codetect-index")
	buildCmd := exec.Command("go", "build", "-o", indexerBin, "./cmd/codetect-index")
	buildCmd.Dir = repoRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build indexer: %v\nOutput: %s", err, output)
	}

	// Step 2: Run indexer on the temp directory
	indexCmd := exec.Command(indexerBin, "index", tmpDir)
	indexCmd.Dir = tmpDir
	if output, err := indexCmd.CombinedOutput(); err != nil {
		t.Fatalf("indexing failed: %v\nOutput: %s", err, output)
	}

	// Verify index was created
	indexPath := filepath.Join(tmpDir, ".codetect", "index.db")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("index database was not created")
	}

	// Step 3: Build the MCP server binary
	mcpBin := filepath.Join(tmpDir, "codetect-mcp")
	buildMcpCmd := exec.Command("go", "build", "-o", mcpBin, "./cmd/codetect")
	buildMcpCmd.Dir = repoRoot
	if output, err := buildMcpCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build MCP server: %v\nOutput: %s", err, output)
	}

	// Step 4: Start MCP server
	mcpCmd := exec.Command(mcpBin)
	mcpCmd.Dir = tmpDir
	stdin, err := mcpCmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin pipe: %v", err)
	}
	stdout, err := mcpCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}

	if err := mcpCmd.Start(); err != nil {
		t.Fatalf("failed to start MCP server: %v", err)
	}

	// Ensure server is killed when test finishes
	defer func() {
		mcpCmd.Process.Kill()
		mcpCmd.Wait()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Step 5: Send initialize request
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	if err := sendRequest(stdin, initReq); err != nil {
		t.Fatalf("initialize request failed: %v", err)
	}

	// Step 6: Send tools/list request
	toolsListReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	response, err := sendRequestAndRead(stdin, stdout, toolsListReq)
	if err != nil {
		t.Fatalf("tools/list request failed: %v", err)
	}

	// Verify tools are registered
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatal("invalid tools/list response format")
	}

	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("tools field missing or invalid")
	}

	expectedTools := map[string]bool{
		"search_keyword":    false,
		"get_file":          false,
		"find_symbol":       false,
		"list_defs_in_file": false,
		"hybrid_search_v2":  false,
	}

	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := toolMap["name"].(string); ok {
			if _, exists := expectedTools[name]; exists {
				expectedTools[name] = true
			}
		}
	}

	// Check all expected tools were found
	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %q not found", name)
		}
	}

	// Step 7: Test search_keyword tool
	searchReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search_keyword",
			"arguments": map[string]any{
				"query": "func main",
				"top_k": 5,
			},
		},
	}

	searchResponse, err := sendRequestAndRead(stdin, stdout, searchReq)
	if err != nil {
		t.Fatalf("search_keyword request failed: %v", err)
	}

	// Verify search returned results containing main.go
	if result, ok := searchResponse["result"].(map[string]any); ok {
		if content, ok := result["content"].([]any); ok && len(content) > 0 {
			if firstContent, ok := content[0].(map[string]any); ok {
				if text, ok := firstContent["text"].(string); ok {
					if !contains(text, "main.go") {
						t.Errorf("expected search results to contain main.go, got: %s", text)
					}
				}
			}
		}
	}

	// Step 8: Test find_symbol tool
	symbolReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "find_symbol",
			"arguments": map[string]any{
				"name":  "Server",
				"limit": 10,
			},
		},
	}

	symbolResponse, err := sendRequestAndRead(stdin, stdout, symbolReq)
	if err != nil {
		t.Fatalf("find_symbol request failed: %v", err)
	}

	// Verify symbol search found Server struct
	if result, ok := symbolResponse["result"].(map[string]any); ok {
		if content, ok := result["content"].([]any); ok && len(content) > 0 {
			if firstContent, ok := content[0].(map[string]any); ok {
				if text, ok := firstContent["text"].(string); ok {
					if !contains(text, "Server") {
						t.Errorf("expected symbol results to contain Server, got: %s", text)
					}
				}
			}
		}
	}

	t.Log("Integration smoke test passed!")
}

// sendRequest sends a JSON-RPC request to stdin
func sendRequest(stdin io.WriteCloser, req map[string]any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	if _, err := stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	return nil
}

// sendRequestAndRead sends a request and reads the response
func sendRequestAndRead(stdin io.WriteCloser, stdout io.ReadCloser, req map[string]any) (map[string]any, error) {
	if err := sendRequest(stdin, req); err != nil {
		return nil, err
	}

	// Read response (with timeout)
	buf := new(bytes.Buffer)
	done := make(chan error, 1)

	go func() {
		b := make([]byte, 4096)
		n, err := stdout.Read(b)
		if err != nil {
			done <- err
			return
		}
		buf.Write(b[:n])
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout reading response")
	}

	var response map[string]any
	if err := json.Unmarshal(buf.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (raw: %s)", err, buf.String())
	}

	return response, nil
}

// contains checks if a string contains a substring (simple helper)
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
