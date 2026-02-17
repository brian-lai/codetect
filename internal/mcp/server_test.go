package mcp

import (
	"encoding/json"
	"testing"
)

func TestHandleInitialize_IncludesInstructions(t *testing.T) {
	server := NewServer("codetect", "3.0.0")

	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := server.handleInitialize(req)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	// Marshal and re-parse to check JSON output
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	instructions, ok := result["instructions"]
	if !ok {
		t.Fatal("InitializeResult must include 'instructions' field")
	}

	instrStr, ok := instructions.(string)
	if !ok || instrStr == "" {
		t.Fatal("instructions must be a non-empty string")
	}

	// Verify instructions mention key guidance
	if len(instrStr) < 50 {
		t.Errorf("instructions too short (%d chars) — should provide meaningful guidance", len(instrStr))
	}
}

func TestHandleInitialize_ServerInfo(t *testing.T) {
	server := NewServer("codetect", "3.0.0")

	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := server.handleInitialize(req)

	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	serverInfo := result["serverInfo"].(map[string]interface{})
	if serverInfo["name"] != "codetect" {
		t.Errorf("expected name 'codetect', got %v", serverInfo["name"])
	}
	if serverInfo["version"] != "3.0.0" {
		t.Errorf("expected version '3.0.0', got %v", serverInfo["version"])
	}
}
