package symbols

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// newTestIndex creates a fresh in-memory SQLite index for testing.
func newTestIndex(t *testing.T) *Index {
	t.Helper()
	tmpDir := t.TempDir()
	idx, err := NewIndex(filepath.Join(tmpDir, "symbols.db"))
	if err != nil {
		t.Fatalf("NewIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	idx.root = "/repo"
	return idx
}

func TestInsertAndQueryCallers(t *testing.T) {
	idx := newTestIndex(t)

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	refs := []SymbolRef{
		{Name: "Update", QualifiedName: "Index.Update", Kind: "call", SourcePath: "cmd/main.go", SourceLine: 42, SourceScope: "main"},
		{Name: "Update", QualifiedName: "Index.Update", Kind: "call", SourcePath: "internal/daemon/daemon.go", SourceLine: 88, SourceScope: "runIndex"},
		{Name: "Close", QualifiedName: "Index.Close", Kind: "call", SourcePath: "cmd/main.go", SourceLine: 10, SourceScope: "main"},
	}
	if err := idx.InsertRefs(tx, refs); err != nil {
		t.Fatalf("InsertRefs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Query by unqualified name
	callers, err := idx.QueryCallers("Update", 10)
	if err != nil {
		t.Fatalf("QueryCallers: %v", err)
	}
	if len(callers) != 2 {
		t.Errorf("expected 2 callers of Update, got %d", len(callers))
	}

	// Query by qualified name
	callers, err = idx.QueryCallers("Index.Update", 10)
	if err != nil {
		t.Fatalf("QueryCallers qualified: %v", err)
	}
	if len(callers) != 2 {
		t.Errorf("expected 2 callers of Index.Update, got %d", len(callers))
	}

	// Query for something not present
	callers, err = idx.QueryCallers("NonExistent", 10)
	if err != nil {
		t.Fatalf("QueryCallers missing: %v", err)
	}
	if len(callers) != 0 {
		t.Errorf("expected 0 callers for NonExistent, got %d", len(callers))
	}
}

func TestQueryRefs_FilterByKind(t *testing.T) {
	idx := newTestIndex(t)

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	refs := []SymbolRef{
		{Name: "DB", Kind: "call", SourcePath: "a.go", SourceLine: 1},
		{Name: "DB", Kind: "type_ref", SourcePath: "b.go", SourceLine: 5},
		{Name: "DB", Kind: "type_ref", SourcePath: "c.go", SourceLine: 9},
	}
	if err := idx.InsertRefs(tx, refs); err != nil {
		t.Fatalf("InsertRefs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// All kinds
	all, err := idx.QueryRefs("DB", "all", 10)
	if err != nil {
		t.Fatalf("QueryRefs all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 total refs, got %d", len(all))
	}

	// Only calls
	calls, err := idx.QueryRefs("DB", "call", 10)
	if err != nil {
		t.Fatalf("QueryRefs call: %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("expected 1 call ref, got %d", len(calls))
	}

	// Only type_refs
	typeRefs, err := idx.QueryRefs("DB", "type_ref", 10)
	if err != nil {
		t.Fatalf("QueryRefs type_ref: %v", err)
	}
	if len(typeRefs) != 2 {
		t.Errorf("expected 2 type_refs, got %d", len(typeRefs))
	}
}

func TestDeleteRefsByFile(t *testing.T) {
	idx := newTestIndex(t)

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	refs := []SymbolRef{
		{Name: "Foo", Kind: "call", SourcePath: "file_a.go", SourceLine: 1},
		{Name: "Bar", Kind: "call", SourcePath: "file_a.go", SourceLine: 2},
		{Name: "Baz", Kind: "call", SourcePath: "file_b.go", SourceLine: 5},
	}
	if err := idx.InsertRefs(tx, refs); err != nil {
		t.Fatalf("InsertRefs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Delete refs for file_a.go only
	tx2, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin2: %v", err)
	}
	if err := idx.DeleteRefsByFile(tx2, "file_a.go"); err != nil {
		t.Fatalf("DeleteRefsByFile: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit2: %v", err)
	}

	// file_a.go refs should be gone
	refsA, err := idx.QueryRefs("Foo", "all", 10)
	if err != nil {
		t.Fatalf("QueryRefs Foo: %v", err)
	}
	if len(refsA) != 0 {
		t.Errorf("expected 0 refs for Foo after delete, got %d", len(refsA))
	}

	// file_b.go refs should still be present
	refsB, err := idx.QueryRefs("Baz", "all", 10)
	if err != nil {
		t.Fatalf("QueryRefs Baz: %v", err)
	}
	if len(refsB) != 1 {
		t.Errorf("expected 1 ref for Baz, got %d", len(refsB))
	}
}

func TestInsertAndQueryImplementations(t *testing.T) {
	idx := newTestIndex(t)

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	relations := []TypeRelation{
		{ChildType: "SQLiteDB", ParentType: "DB", Relation: "implements", Path: "internal/db/sqlite.go", Line: 15},
		{ChildType: "PostgresDB", ParentType: "DB", Relation: "implements", Path: "internal/db/postgres.go", Line: 20},
		{ChildType: "MockDB", ParentType: "DB", Relation: "implements", Path: "internal/db/mock.go", Line: 5},
	}
	if err := idx.InsertTypeRelations(tx, relations); err != nil {
		t.Fatalf("InsertTypeRelations: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	impls, err := idx.QueryImplementations("DB", 10)
	if err != nil {
		t.Fatalf("QueryImplementations: %v", err)
	}
	if len(impls) != 3 {
		t.Errorf("expected 3 implementations of DB, got %d", len(impls))
	}

	// Verify all are "implements"
	for _, impl := range impls {
		if impl.Relation != "implements" {
			t.Errorf("expected relation=implements, got %q", impl.Relation)
		}
		if impl.ParentType != "DB" {
			t.Errorf("expected parentType=DB, got %q", impl.ParentType)
		}
	}

	// Unknown parent returns empty, not error
	none, err := idx.QueryImplementations("NonExistentInterface", 10)
	if err != nil {
		t.Fatalf("QueryImplementations missing: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 impls, got %d", len(none))
	}
}

func TestDeleteTypeRelationsByFile(t *testing.T) {
	idx := newTestIndex(t)

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	relations := []TypeRelation{
		{ChildType: "Foo", ParentType: "Base", Relation: "extends", Path: "pkg/foo.go", Line: 3},
		{ChildType: "Bar", ParentType: "Base", Relation: "extends", Path: "pkg/bar.go", Line: 7},
	}
	if err := idx.InsertTypeRelations(tx, relations); err != nil {
		t.Fatalf("InsertTypeRelations: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin2: %v", err)
	}
	if err := idx.DeleteTypeRelationsByFile(tx2, "pkg/foo.go"); err != nil {
		t.Fatalf("DeleteTypeRelationsByFile: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit2: %v", err)
	}

	remaining, err := idx.QueryImplementations("Base", 10)
	if err != nil {
		t.Fatalf("QueryImplementations: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining relation, got %d", len(remaining))
	}
	if remaining[0].ChildType != "Bar" {
		t.Errorf("expected Bar to remain, got %q", remaining[0].ChildType)
	}
}

func TestInsertRefs_DuplicateUpserts(t *testing.T) {
	idx := newTestIndex(t)

	// Insert a ref, then insert the same (path, line, name) with updated fields.
	// The upsert should update the existing row rather than error or duplicate.
	ref := SymbolRef{Name: "Update", QualifiedName: "Index.Update", Kind: "call", SourcePath: "cmd/main.go", SourceLine: 42}

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := idx.InsertRefs(tx, []SymbolRef{ref}); err != nil {
		t.Fatalf("first InsertRefs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Re-insert same (path, line, name) with updated qualified_name
	ref.QualifiedName = "symbols.Index.Update"
	tx2, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin2: %v", err)
	}
	if err := idx.InsertRefs(tx2, []SymbolRef{ref}); err != nil {
		t.Fatalf("second InsertRefs (upsert): %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit2: %v", err)
	}

	// Should still be exactly 1 row, with updated qualified_name
	results, err := idx.QueryCallers("Update", 10)
	if err != nil {
		t.Fatalf("QueryCallers: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 row after upsert, got %d", len(results))
	}
	if results[0].QualifiedName != "symbols.Index.Update" {
		t.Errorf("expected updated qualified_name, got %q", results[0].QualifiedName)
	}
}

func TestQueryCallers_ResultFields(t *testing.T) {
	idx := newTestIndex(t)

	tx, err := idx.adapter.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	refs := []SymbolRef{
		{Name: "Update", QualifiedName: "Index.Update", Kind: "call", SourcePath: "cmd/main.go", SourceLine: 42, SourceScope: "main"},
	}
	if err := idx.InsertRefs(tx, refs); err != nil {
		t.Fatalf("InsertRefs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	callers, err := idx.QueryCallers("Update", 10)
	if err != nil {
		t.Fatalf("QueryCallers: %v", err)
	}
	if len(callers) != 1 {
		t.Fatalf("expected 1 caller, got %d", len(callers))
	}
	c := callers[0]
	if c.Name != "Update" {
		t.Errorf("Name: got %q, want %q", c.Name, "Update")
	}
	if c.QualifiedName != "Index.Update" {
		t.Errorf("QualifiedName: got %q, want %q", c.QualifiedName, "Index.Update")
	}
	if c.Kind != "call" {
		t.Errorf("Kind: got %q, want %q", c.Kind, "call")
	}
	if c.SourcePath != "cmd/main.go" {
		t.Errorf("SourcePath: got %q, want %q", c.SourcePath, "cmd/main.go")
	}
	if c.SourceLine != 42 {
		t.Errorf("SourceLine: got %d, want %d", c.SourceLine, 42)
	}
	if c.SourceScope != "main" {
		t.Errorf("SourceScope: got %q, want %q", c.SourceScope, "main")
	}
}

func TestMigrateV2ToV3(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "v2.db")

	// Manually create a v2 database (symbols + files tables, version=2)
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := rawDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("WAL: %v", err)
	}
	v2Schema := `
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL,
    path TEXT NOT NULL, line INTEGER NOT NULL, language TEXT,
    pattern TEXT, scope TEXT, signature TEXT,
    UNIQUE(repo_root, name, path, line)
);
CREATE TABLE IF NOT EXISTS files (
    repo_root TEXT NOT NULL, path TEXT NOT NULL,
    mtime INTEGER NOT NULL, size INTEGER NOT NULL, indexed_at INTEGER NOT NULL,
    PRIMARY KEY (repo_root, path)
);
INSERT INTO schema_version (version) VALUES (2);`
	if _, err := rawDB.Exec(v2Schema); err != nil {
		rawDB.Close()
		t.Fatalf("creating v2 schema: %v", err)
	}
	rawDB.Close()

	// Now open with NewIndex — should trigger migration to v3
	idx, err := NewIndex(dbPath)
	if err != nil {
		t.Fatalf("NewIndex (migration): %v", err)
	}
	defer idx.Close()

	// Verify schema_version is now 3
	var version int
	if err := idx.sqlDB.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if version != 3 {
		t.Errorf("expected schema version 3 after migration, got %d", version)
	}

	// Verify symbol_refs table exists
	var tableName string
	err = idx.sqlDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='symbol_refs'").Scan(&tableName)
	if err != nil || tableName != "symbol_refs" {
		t.Errorf("symbol_refs table not found after migration: err=%v name=%q", err, tableName)
	}

	// Verify type_relations table exists
	err = idx.sqlDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='type_relations'").Scan(&tableName)
	if err != nil || tableName != "type_relations" {
		t.Errorf("type_relations table not found after migration: err=%v name=%q", err, tableName)
	}
}
