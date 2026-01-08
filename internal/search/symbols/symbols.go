package symbols

// Phase 2 stub for symbol indexing via universal-ctags

import (
	"fmt"
)

// Symbol represents a code symbol (function, type, variable, etc.)
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`      // function, type, variable, etc.
	Path     string `json:"path"`      // file path
	Line     int    `json:"line"`      // 1-indexed line number
	Language string `json:"language"`  // detected language
	Pattern  string `json:"pattern"`   // search pattern (ctags output)
	Scope    string `json:"scope"`     // parent scope (e.g., class name)
}

// Index is the SQLite-backed symbol index
// Phase 2: Will be populated by universal-ctags
type Index struct {
	dbPath string
}

// NewIndex creates a new symbol index at the given path
func NewIndex(dbPath string) (*Index, error) {
	// Phase 2: Initialize SQLite database
	return &Index{dbPath: dbPath}, nil
}

// FindSymbol searches for symbols by name
// Phase 2: Query SQLite for matching symbols
func (idx *Index) FindSymbol(name string, limit int) ([]Symbol, error) {
	return nil, fmt.Errorf("symbol search not implemented (Phase 2)")
}

// ListDefsInFile returns all symbol definitions in a file
// Phase 2: Query SQLite filtered by path
func (idx *Index) ListDefsInFile(path string) ([]Symbol, error) {
	return nil, fmt.Errorf("symbol listing not implemented (Phase 2)")
}

// Update re-indexes files that have changed since last index
// Phase 2: Check mtime, invoke ctags, update SQLite
func (idx *Index) Update(root string) error {
	return fmt.Errorf("symbol indexing not implemented (Phase 2)")
}

// Schema for the SQLite symbol table (Phase 2)
//
// CREATE TABLE IF NOT EXISTS symbols (
//     id INTEGER PRIMARY KEY AUTOINCREMENT,
//     name TEXT NOT NULL,
//     kind TEXT NOT NULL,
//     path TEXT NOT NULL,
//     line INTEGER NOT NULL,
//     language TEXT,
//     pattern TEXT,
//     scope TEXT,
//     UNIQUE(name, path, line)
// );
//
// CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
// CREATE INDEX IF NOT EXISTS idx_symbols_path ON symbols(path);
// CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);
//
// CREATE TABLE IF NOT EXISTS files (
//     path TEXT PRIMARY KEY,
//     mtime INTEGER NOT NULL,
//     size INTEGER NOT NULL,
//     hash TEXT
// );
