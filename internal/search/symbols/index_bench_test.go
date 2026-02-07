package symbols

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkIndexingAstGrep benchmarks indexing with ast-grep
func BenchmarkIndexingAstGrep(b *testing.B) {
	if !AstGrepAvailable() {
		b.Skip("ast-grep not available")
	}

	// Use codetect's own codebase for benchmarking
	cwd, _ := os.Getwd()
	repoRoot := filepath.Join(cwd, "../../..")

	// Create temp db for each iteration
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	// Force hybrid mode
	oldEnv := os.Getenv("CODETECT_INDEX_BACKEND")
	os.Setenv("CODETECT_INDEX_BACKEND", "auto")
	defer func() {
		if oldEnv != "" {
			os.Setenv("CODETECT_INDEX_BACKEND", oldEnv)
		} else {
			os.Unsetenv("CODETECT_INDEX_BACKEND")
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx, err := NewIndex(dbPath)
		if err != nil {
			b.Fatalf("Creating index: %v", err)
		}

		if err := idx.Update(repoRoot); err != nil {
			b.Fatalf("Indexing: %v", err)
		}

		idx.Close()
	}
}

// BenchmarkBatchInsert benchmarks symbol insertion with different batch sizes
func BenchmarkBatchInsert(b *testing.B) {
	// Create a large set of test symbols
	const numSymbols = 10000
	symbols := make([]Symbol, numSymbols)
	for i := 0; i < numSymbols; i++ {
		symbols[i] = Symbol{
			Name: "testFunc",
			Kind: "function",
			Path: "test.go",
			Line: i + 1,
		}
	}

	batchSizes := []int{1, 100, 500, 1000, 5000}

	for _, batchSize := range batchSizes {
		b.Run(string(rune(batchSize)), func(b *testing.B) {
			tmpDir := b.TempDir()
			dbPath := filepath.Join(tmpDir, "bench.db")

			idx, err := NewIndex(dbPath)
			if err != nil {
				b.Fatalf("Creating index: %v", err)
			}
			defer idx.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Clear symbols table
				idx.adapter.Exec("DELETE FROM symbols")

				// Start transaction
				tx, _ := idx.adapter.Begin()
				b.StartTimer()

				// Batch insert
				if err := idx.batchInsertSymbols(tx, symbols, batchSize); err != nil {
					b.Fatalf("Batch insert: %v", err)
				}

				b.StopTimer()
				tx.Commit()
			}
		})
	}
}
