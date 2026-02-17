package tools

import (
	"sync"
	"testing"
)

func TestNewResourcePool(t *testing.T) {
	pool := NewResourcePool("/tmp/test-repo")
	if pool == nil {
		t.Fatal("NewResourcePool should not return nil")
	}
	if pool.RepoRoot() != "/tmp/test-repo" {
		t.Errorf("expected repoRoot '/tmp/test-repo', got %q", pool.RepoRoot())
	}
}

func TestResourcePool_LazyInit(t *testing.T) {
	// Pool should not open any resources until first access
	pool := NewResourcePool("/tmp/nonexistent-repo")
	defer pool.Close()

	// Just creating the pool should not error, even with bad path
	if pool == nil {
		t.Fatal("pool should be non-nil even with nonexistent path")
	}
}

func TestResourcePool_Close_Idempotent(t *testing.T) {
	pool := NewResourcePool("/tmp/test-repo")

	// Close should not panic even when called multiple times
	pool.Close()
	pool.Close()
	pool.Close()
}

func TestResourcePool_ConcurrentAccess(t *testing.T) {
	pool := NewResourcePool("/tmp/test-repo")
	defer pool.Close()

	// Multiple goroutines accessing pool should not race
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// These will error (no DB), but must not panic or race
			pool.SymbolIndex()
		}()
	}
	wg.Wait()
}

func TestResourcePool_SymbolIndex_ErrorsWithoutDB(t *testing.T) {
	// Force SQLite mode so the file existence check triggers
	t.Setenv("CODETECT_DB_TYPE", "sqlite")
	t.Setenv("CODETECT_DB_DSN", "")

	pool := NewResourcePool("/tmp/nonexistent-repo")
	defer pool.Close()

	_, err := pool.SymbolIndex()
	if err == nil {
		t.Error("SymbolIndex should error when no DB exists")
	}
}

func TestResourcePool_ReusesResources(t *testing.T) {
	// This test requires a real repo with a DB.
	// Skip when DB not available.
	t.Skip("requires real repository with .codetect/symbols.db")

	pool := NewResourcePool(".")
	defer pool.Close()

	idx1, err := pool.SymbolIndex()
	if err != nil {
		t.Skipf("SymbolIndex unavailable: %v", err)
	}

	idx2, err := pool.SymbolIndex()
	if err != nil {
		t.Fatalf("second SymbolIndex call failed: %v", err)
	}

	if idx1 != idx2 {
		t.Error("SymbolIndex should return the same instance on second call")
	}
}
