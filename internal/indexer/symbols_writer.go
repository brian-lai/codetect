// Package indexer — symbol-writing bridge that populates the `symbols` table
// directly from chunker metadata. Replaces the v1 ast-grep/ctags subprocess
// path for the default installation.
//
// STUB — phase 2 of plan 2026-05-01-codetect-tier1-unbreak.
// Contract: context/data/2026-05-01-codetect-tier1-unbreak-spec.md §2.
package indexer

import (
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/search/symbols"
)

// SymbolsWriter writes symbol rows into the `symbols` table using the same
// schema and upsert path as the v1 symbols.Index, but driven from the chunker's
// per-chunk metadata (NodeName, ScopeKind, ParentScope, ...).
//
// Owns: the `symbols` and `files` tables inside the shared index.db.
// Does not own: chunk_locations, embedding_cache, failed_chunks.
type SymbolsWriter struct {
	database db.DB
	dialect  db.Dialect
	repoRoot string
}

// NewSymbolsWriter ensures the symbols schema exists and returns a writer bound
// to repoRoot. The schema DDL is the same used by symbols.NewIndexWithConfig.
func NewSymbolsWriter(database db.DB, dialect db.Dialect, repoRoot string) (*SymbolsWriter, error) {
	panic("not implemented: internal/indexer.NewSymbolsWriter")
}

// ReplaceForFiles atomically clears existing symbol rows for the given
// (repo_root, path) pairs and inserts new rows derived from the chunks passed
// in. Intended to be called once per batch inside Indexer.processBatch after
// chunk_locations are written.
//
// Takes []embedding.Chunk (not chunker.Chunk) because that is the type that
// actually flows through processBatch. The embedding.Chunk type carries the
// symbol-relevant fields (Path, StartLine, EndLine, NodeName, NodeType,
// ParentScope, ScopeKind, ReceiverType, Language) that are preserved from
// the chunker output at the projection in indexer.go:processBatch.
//
// paths: relative paths that were (re-)chunked in this call. MUST include every
//        file whose symbols should be cleared, including files that produced no
//        named chunks (e.g., a file whose last named function was deleted).
// chunks: all chunks produced by those files. chunks whose NodeName == "" are
//         skipped. A chunk's Path must appear in the paths slice; chunks with
//         unknown Path are dropped (defensively — not an error).
//
// Invariant: `paths` is the authority for what gets deleted; `chunks` is the
// authority for what gets inserted. Passing a non-empty paths slice with no
// chunks for that path is valid and clears symbols for that path.
func (w *SymbolsWriter) ReplaceForFiles(paths []string, chunks []embedding.Chunk) error {
	panic("not implemented: internal/indexer.SymbolsWriter.ReplaceForFiles")
}

// DropForPaths removes all symbol rows for the given deleted files. Called for
// each entry in Indexer's filesToDelete list.
func (w *SymbolsWriter) DropForPaths(paths []string) error {
	panic("not implemented: internal/indexer.SymbolsWriter.DropForPaths")
}

// ClearRepo removes every symbol row for this writer's repoRoot. Used by the
// --force path at the start of a full reindex.
func (w *SymbolsWriter) ClearRepo() error {
	panic("not implemented: internal/indexer.SymbolsWriter.ClearRepo")
}

// mapChunkToSymbol converts an embedding.Chunk into a symbols.Symbol row, or
// returns ok=false if the chunk does not represent a named definition.
// Centralizes the mapping rules defined in spec §2.3.
//
// Kind derivation: prefer chunker.mapNodeTypeToKind(c.NodeType) — that is the
// kind of THIS node (e.g. "method" for a Go method). c.ScopeKind is the kind
// of the CONTAINING scope (e.g. "struct" for a method's receiver type) and
// is only used when NodeType is empty (defensive fallback).
func mapChunkToSymbol(c embedding.Chunk, repoRoot string) (_ symbols.Symbol, ok bool) {
	panic("not implemented: internal/indexer.mapChunkToSymbol")
}
