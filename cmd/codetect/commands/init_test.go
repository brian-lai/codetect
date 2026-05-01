package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"codetect/cmd/codetect/commands"
)

func TestRunInit_CreatesMcpJson(t *testing.T) {
	dir := t.TempDir()
	// Change to temp dir for the duration of this test
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	code := commands.RunInit(nil)
	if code != commands.ExitOK {
		t.Errorf("RunInit: got exit %d, want 0", code)
	}

	dest := filepath.Join(dir, ".mcp.json")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf(".mcp.json not created: %v", err)
	}
	if len(data) == 0 {
		t.Error(".mcp.json is empty")
	}
}

func TestRunInit_ExistingFile_Exits1(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Create the file first
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("{}"), 0644)

	code := commands.RunInit(nil)
	if code != commands.ExitError {
		t.Errorf("RunInit with existing file: got exit %d, want ExitError", code)
	}
}

func TestRunInit_Force_Overwrites(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Create the file first with different content
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("old-content"), 0644)

	code := commands.RunInit([]string{"--force"})
	if code != commands.ExitOK {
		t.Errorf("RunInit --force: got exit %d, want 0", code)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "old-content" {
		t.Error("RunInit --force: file was not overwritten")
	}
}
