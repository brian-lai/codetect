package tools

import (
	"testing"

	"codetect/internal/mcp"
)

func TestSymbolsTool_Registered(t *testing.T) {
	server := mcp.NewServer("test", "1.0")
	config := &Config{
		Pool: NewResourcePool("/tmp/nonexistent"),
	}

	RegisterSymbolTools(server, config)

	tools := server.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "symbols" {
		t.Errorf("expected tool name 'symbols', got %q", tools[0].Name)
	}
}

func TestSymbolsTool_FindMode_MissingName(t *testing.T) {
	server := mcp.NewServer("test", "1.0")
	config := &Config{
		Pool: NewResourcePool("."),
	}

	RegisterSymbolTools(server, config)

	// find mode (default) with no name should error
	_, err := server.CallTool("symbols", map[string]any{})
	if err == nil {
		t.Error("expected error for find mode without name")
	}
}

func TestSymbolsTool_ListMode_MissingPath(t *testing.T) {
	server := mcp.NewServer("test", "1.0")
	config := &Config{
		Pool: NewResourcePool("."),
	}

	RegisterSymbolTools(server, config)

	_, err := server.CallTool("symbols", map[string]any{"mode": "list"})
	if err == nil {
		t.Error("expected error for list mode without path")
	}
}

func TestSymbolsTool_InvalidMode(t *testing.T) {
	server := mcp.NewServer("test", "1.0")
	config := &Config{
		Pool: NewResourcePool("."),
	}

	RegisterSymbolTools(server, config)

	_, err := server.CallTool("symbols", map[string]any{"mode": "bogus"})
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestSymbolsTool_NilPool(t *testing.T) {
	server := mcp.NewServer("test", "1.0")
	config := &Config{Pool: nil}

	RegisterSymbolTools(server, config)

	_, err := server.CallTool("symbols", map[string]any{"name": "foo"})
	if err == nil {
		t.Error("expected error when pool is nil")
	}
}

func TestSymbolsTool_DefaultMode_IsFind(t *testing.T) {
	server := mcp.NewServer("test", "1.0")
	config := &Config{
		Pool: NewResourcePool("."),
	}

	RegisterSymbolTools(server, config)

	// Not providing mode should default to find, which needs name
	_, err := server.CallTool("symbols", map[string]any{})
	if err == nil {
		t.Error("expected error for default find mode without name")
	}
}
