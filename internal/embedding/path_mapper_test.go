package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPathMapper_HashAndResolve(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPathMapper(filepath.Join(dir, "path_map.json"))
	if err != nil {
		t.Fatalf("NewPathMapper: %v", err)
	}

	path := "/home/user/project/src/main.go"
	hash := pm.HashPath(path)

	// Hash should be a valid SHA-256 hex string
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars: %s", len(hash), hash)
	}

	// Verify hash is correct
	expected := sha256.Sum256([]byte(path))
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash mismatch: got %s, want %s", hash, expectedHex)
	}

	// Resolve should return the original path
	resolved, ok := pm.ResolvePath(hash)
	if !ok {
		t.Fatal("ResolvePath returned false")
	}
	if resolved != path {
		t.Errorf("ResolvePath: got %s, want %s", resolved, path)
	}

	// Unknown hash should return false
	_, ok = pm.ResolvePath("unknown-hash")
	if ok {
		t.Error("ResolvePath should return false for unknown hash")
	}
}

func TestPathMapper_Idempotent(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPathMapper(filepath.Join(dir, "path_map.json"))
	if err != nil {
		t.Fatalf("NewPathMapper: %v", err)
	}

	path := "src/main.go"
	hash1 := pm.HashPath(path)
	hash2 := pm.HashPath(path)

	if hash1 != hash2 {
		t.Errorf("HashPath not idempotent: %s vs %s", hash1, hash2)
	}
}

func TestPathMapper_Persistence(t *testing.T) {
	dir := t.TempDir()
	mapFile := filepath.Join(dir, "path_map.json")

	// Create mapper and add some paths
	pm1, err := NewPathMapper(mapFile)
	if err != nil {
		t.Fatalf("NewPathMapper: %v", err)
	}

	paths := []string{
		"/repo/src/main.go",
		"/repo/src/util.go",
		"/repo/README.md",
	}
	hashes := make([]string, len(paths))
	for i, p := range paths {
		hashes[i] = pm1.HashPath(p)
	}

	// Flush to disk
	if err := pm1.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(mapFile); err != nil {
		t.Fatalf("sidecar file not created: %v", err)
	}

	// Create new mapper from same file
	pm2, err := NewPathMapper(mapFile)
	if err != nil {
		t.Fatalf("NewPathMapper (reload): %v", err)
	}

	// All paths should be resolvable
	for i, hash := range hashes {
		resolved, ok := pm2.ResolvePath(hash)
		if !ok {
			t.Errorf("ResolvePath(%s) returned false after reload", hash[:12])
			continue
		}
		if resolved != paths[i] {
			t.Errorf("ResolvePath: got %s, want %s", resolved, paths[i])
		}
	}
}

func TestPathMapper_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPathMapper(filepath.Join(dir, "path_map.json"))
	if err != nil {
		t.Fatalf("NewPathMapper: %v", err)
	}

	var wg sync.WaitGroup
	const n = 100

	// Concurrent writes
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join("/repo", "file"+string(rune('A'+i%26))+".go")
			hash := pm.HashPath(path)
			resolved, ok := pm.ResolvePath(hash)
			if !ok {
				t.Errorf("ResolvePath failed for %s", path)
				return
			}
			if resolved != path {
				t.Errorf("ResolvePath: got %s, want %s", resolved, path)
			}
		}(i)
	}

	wg.Wait()
}

func TestPathMapper_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPathMapper(filepath.Join(dir, "new_map.json"))
	if err != nil {
		t.Fatalf("NewPathMapper: %v", err)
	}

	// Should work with no existing file
	hash := pm.HashPath("test/path.go")
	if hash == "" {
		t.Error("HashPath returned empty string")
	}
}
