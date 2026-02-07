package symbols

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/db"

	_ "modernc.org/sqlite"
)

const schemaVersion = 3

const schema = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    language TEXT,
    pattern TEXT,
    scope TEXT,
    signature TEXT,
    UNIQUE(repo_root, name, path, line)
);

CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_path ON symbols(path);
CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);
CREATE INDEX IF NOT EXISTS idx_symbols_repo_path ON symbols(repo_root, path);

CREATE TABLE IF NOT EXISTS files (
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    mtime INTEGER NOT NULL,
    size INTEGER NOT NULL,
    indexed_at INTEGER NOT NULL,
    PRIMARY KEY (repo_root, path)
);

CREATE TABLE IF NOT EXISTS symbol_refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    name TEXT NOT NULL,
    qualified_name TEXT,
    kind TEXT NOT NULL,
    source_path TEXT NOT NULL,
    source_line INTEGER NOT NULL,
    source_scope TEXT,
    UNIQUE(repo_root, source_path, source_line, name)
);

CREATE INDEX IF NOT EXISTS idx_refs_name ON symbol_refs(repo_root, name);
CREATE INDEX IF NOT EXISTS idx_refs_qualified ON symbol_refs(repo_root, qualified_name);
CREATE INDEX IF NOT EXISTS idx_refs_source ON symbol_refs(repo_root, source_path);
CREATE INDEX IF NOT EXISTS idx_refs_scope ON symbol_refs(repo_root, source_scope);

CREATE TABLE IF NOT EXISTS type_relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    child_type TEXT NOT NULL,
    parent_type TEXT NOT NULL,
    relation TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    UNIQUE(repo_root, child_type, parent_type, path)
);

CREATE INDEX IF NOT EXISTS idx_types_child ON type_relations(repo_root, child_type);
CREATE INDEX IF NOT EXISTS idx_types_parent ON type_relations(repo_root, parent_type);
`

// OpenDB opens or creates the symbol database at the given path
func OpenDB(dbPath string) (*sql.DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	// Initialize schema if needed
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return db, nil
}

func initSchema(db *sql.DB) error {
	// Check current schema version
	var version int
	err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		// Fresh database, create schema
		if _, err := db.Exec(schema); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
		return nil
	}
	if err != nil {
		// Table doesn't exist, create schema
		if _, err := db.Exec(schema); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
		return nil
	}

	// Version exists, check for migrations
	if version < schemaVersion {
		// Migration from version 2 to 3: Add symbol_refs and type_relations tables
		if version == 2 {
			// Add symbol_refs table
			symbolRefsTable := `
CREATE TABLE IF NOT EXISTS symbol_refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    name TEXT NOT NULL,
    qualified_name TEXT,
    kind TEXT NOT NULL,
    source_path TEXT NOT NULL,
    source_line INTEGER NOT NULL,
    source_scope TEXT,
    UNIQUE(repo_root, source_path, source_line, name)
);
CREATE INDEX IF NOT EXISTS idx_refs_name ON symbol_refs(repo_root, name);
CREATE INDEX IF NOT EXISTS idx_refs_qualified ON symbol_refs(repo_root, qualified_name);
CREATE INDEX IF NOT EXISTS idx_refs_source ON symbol_refs(repo_root, source_path);
CREATE INDEX IF NOT EXISTS idx_refs_scope ON symbol_refs(repo_root, source_scope);
`
			if _, err := db.Exec(symbolRefsTable); err != nil {
				return fmt.Errorf("migrating to v3 (symbol_refs): %w", err)
			}

			// Add type_relations table
			typeRelationsTable := `
CREATE TABLE IF NOT EXISTS type_relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    child_type TEXT NOT NULL,
    parent_type TEXT NOT NULL,
    relation TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    UNIQUE(repo_root, child_type, parent_type, path)
);
CREATE INDEX IF NOT EXISTS idx_types_child ON type_relations(repo_root, child_type);
CREATE INDEX IF NOT EXISTS idx_types_parent ON type_relations(repo_root, parent_type);
`
			if _, err := db.Exec(typeRelationsTable); err != nil {
				return fmt.Errorf("migrating to v3 (type_relations): %w", err)
			}
		}

		// Update schema version
		if _, err := db.Exec("UPDATE schema_version SET version = ?", schemaVersion); err != nil {
			return fmt.Errorf("updating schema version: %w", err)
		}
	}

	return nil
}

// ClearSymbols removes all symbols for a given file path
func ClearSymbols(db *sql.DB, path string) error {
	_, err := db.Exec("DELETE FROM symbols WHERE path = ?", path)
	return err
}

// ClearAllSymbols removes all symbols from the database
func ClearAllSymbols(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM symbols")
	return err
}

// initSchemaWithAdapter initializes the database schema using the adapter and dialect.
// This supports multiple database backends (SQLite, PostgreSQL) by using dialect-aware DDL.
func initSchemaWithAdapter(adapter db.DB, dialect db.Dialect) error {
	// Run dialect-specific initialization statements (e.g., WAL mode for SQLite, pgvector extension for Postgres)
	for _, stmt := range dialect.InitStatements() {
		if _, err := adapter.Exec(stmt); err != nil {
			return fmt.Errorf("init statement %q: %w", stmt, err)
		}
	}

	// Create schema_version table
	schemaVersionColumns := []db.ColumnDef{
		{Name: "version", Type: db.ColTypeInteger, Nullable: false},
	}
	if _, err := adapter.Exec(dialect.CreateTableSQL("schema_version", schemaVersionColumns)); err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	// Check current schema version
	var version int
	err := adapter.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	needsSchema := err != nil // Either no rows or table was just created

	if needsSchema {
		// Create symbols table with repo_root for multi-repo isolation
		symbolColumns := []db.ColumnDef{
			{Name: "id", Type: db.ColTypeAutoIncrement},
			{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
			{Name: "name", Type: db.ColTypeText, Nullable: false},
			{Name: "kind", Type: db.ColTypeText, Nullable: false},
			{Name: "path", Type: db.ColTypeText, Nullable: false},
			{Name: "line", Type: db.ColTypeInteger, Nullable: false},
			{Name: "language", Type: db.ColTypeText, Nullable: true},
			{Name: "pattern", Type: db.ColTypeText, Nullable: true},
			{Name: "scope", Type: db.ColTypeText, Nullable: true},
			{Name: "signature", Type: db.ColTypeText, Nullable: true},
		}
		if _, err := adapter.Exec(dialect.CreateTableSQL("symbols", symbolColumns)); err != nil {
			return fmt.Errorf("creating symbols table: %w", err)
		}

		// Create unique constraint on symbols including repo_root
		uniqueIdxSQL := dialect.CreateIndexSQL("symbols", "idx_symbols_unique", []string{"repo_root", "name", "path", "line"}, true)
		if _, err := adapter.Exec(uniqueIdxSQL); err != nil {
			// Ignore error if index already exists (some databases don't support IF NOT EXISTS for unique constraints)
		}

		// Create indexes on symbols table
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_name", []string{"name"}, false)); err != nil {
			return fmt.Errorf("creating name index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_path", []string{"path"}, false)); err != nil {
			return fmt.Errorf("creating path index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_kind", []string{"kind"}, false)); err != nil {
			return fmt.Errorf("creating kind index: %w", err)
		}
		// Composite index for repo-scoped queries
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_repo_path", []string{"repo_root", "path"}, false)); err != nil {
			return fmt.Errorf("creating repo_path index: %w", err)
		}

		// Create files table with repo_root for multi-repo isolation
		// Use unique index instead of composite PK for dialect compatibility
		fileColumns := []db.ColumnDef{
			{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
			{Name: "path", Type: db.ColTypeText, Nullable: false},
			{Name: "mtime", Type: db.ColTypeInteger, Nullable: false},
			{Name: "size", Type: db.ColTypeInteger, Nullable: false},
			{Name: "indexed_at", Type: db.ColTypeInteger, Nullable: false},
		}
		if _, err := adapter.Exec(dialect.CreateTableSQL("files", fileColumns)); err != nil {
			return fmt.Errorf("creating files table: %w", err)
		}
		// Create unique constraint for files (repo_root, path)
		filesUniqueIdx := dialect.CreateIndexSQL("files", "idx_files_unique", []string{"repo_root", "path"}, true)
		if _, err := adapter.Exec(filesUniqueIdx); err != nil {
			// Ignore error if index already exists
		}

		// Create symbol_refs table (Phase 2b) for storing references (calls, type usages)
		symbolRefsColumns := []db.ColumnDef{
			{Name: "id", Type: db.ColTypeAutoIncrement},
			{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
			{Name: "name", Type: db.ColTypeText, Nullable: false},
			{Name: "qualified_name", Type: db.ColTypeText, Nullable: true},
			{Name: "kind", Type: db.ColTypeText, Nullable: false},
			{Name: "source_path", Type: db.ColTypeText, Nullable: false},
			{Name: "source_line", Type: db.ColTypeInteger, Nullable: false},
			{Name: "source_scope", Type: db.ColTypeText, Nullable: true},
		}
		if _, err := adapter.Exec(dialect.CreateTableSQL("symbol_refs", symbolRefsColumns)); err != nil {
			return fmt.Errorf("creating symbol_refs table: %w", err)
		}

		// Create unique constraint on symbol_refs (repo_root, source_path, source_line, name)
		symbolRefsUniqueIdx := dialect.CreateIndexSQL("symbol_refs", "idx_symbol_refs_unique", []string{"repo_root", "source_path", "source_line", "name"}, true)
		if _, err := adapter.Exec(symbolRefsUniqueIdx); err != nil {
			// Ignore error if index already exists
		}

		// Create indexes on symbol_refs table
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_name", []string{"repo_root", "name"}, false)); err != nil {
			return fmt.Errorf("creating refs name index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_qualified", []string{"repo_root", "qualified_name"}, false)); err != nil {
			return fmt.Errorf("creating refs qualified index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_source", []string{"repo_root", "source_path"}, false)); err != nil {
			return fmt.Errorf("creating refs source index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_scope", []string{"repo_root", "source_scope"}, false)); err != nil {
			return fmt.Errorf("creating refs scope index: %w", err)
		}

		// Create type_relations table (Phase 2b) for storing type relationships (implements, extends, embeds)
		typeRelationsColumns := []db.ColumnDef{
			{Name: "id", Type: db.ColTypeAutoIncrement},
			{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
			{Name: "child_type", Type: db.ColTypeText, Nullable: false},
			{Name: "parent_type", Type: db.ColTypeText, Nullable: false},
			{Name: "relation", Type: db.ColTypeText, Nullable: false},
			{Name: "path", Type: db.ColTypeText, Nullable: false},
			{Name: "line", Type: db.ColTypeInteger, Nullable: false},
		}
		if _, err := adapter.Exec(dialect.CreateTableSQL("type_relations", typeRelationsColumns)); err != nil {
			return fmt.Errorf("creating type_relations table: %w", err)
		}

		// Create unique constraint on type_relations (repo_root, child_type, parent_type, path)
		typeRelationsUniqueIdx := dialect.CreateIndexSQL("type_relations", "idx_type_relations_unique", []string{"repo_root", "child_type", "parent_type", "path"}, true)
		if _, err := adapter.Exec(typeRelationsUniqueIdx); err != nil {
			// Ignore error if index already exists
		}

		// Create indexes on type_relations table
		if _, err := adapter.Exec(dialect.CreateIndexSQL("type_relations", "idx_types_child", []string{"repo_root", "child_type"}, false)); err != nil {
			return fmt.Errorf("creating types child index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("type_relations", "idx_types_parent", []string{"repo_root", "parent_type"}, false)); err != nil {
			return fmt.Errorf("creating types parent index: %w", err)
		}

		// Insert schema version
		insertVersionSQL := fmt.Sprintf("INSERT INTO schema_version (version) VALUES (%s)", dialect.Placeholder(1))
		if _, err := adapter.Exec(insertVersionSQL, schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
	} else if version < schemaVersion {
		// Migration from version 2 to 3: Add symbol_refs and type_relations tables
		if version == 2 {
			// Add symbol_refs table
			symbolRefsColumns := []db.ColumnDef{
				{Name: "id", Type: db.ColTypeAutoIncrement},
				{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
				{Name: "name", Type: db.ColTypeText, Nullable: false},
				{Name: "qualified_name", Type: db.ColTypeText, Nullable: true},
				{Name: "kind", Type: db.ColTypeText, Nullable: false},
				{Name: "source_path", Type: db.ColTypeText, Nullable: false},
				{Name: "source_line", Type: db.ColTypeInteger, Nullable: false},
				{Name: "source_scope", Type: db.ColTypeText, Nullable: true},
			}
			if _, err := adapter.Exec(dialect.CreateTableSQL("symbol_refs", symbolRefsColumns)); err != nil {
				return fmt.Errorf("migrating to v3 (symbol_refs table): %w", err)
			}

			// Create indexes on symbol_refs
			if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_symbol_refs_unique", []string{"repo_root", "source_path", "source_line", "name"}, true)); err != nil {
				// Ignore if exists
			}
			if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_name", []string{"repo_root", "name"}, false)); err != nil {
				return fmt.Errorf("migrating to v3 (refs name index): %w", err)
			}
			if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_qualified", []string{"repo_root", "qualified_name"}, false)); err != nil {
				return fmt.Errorf("migrating to v3 (refs qualified index): %w", err)
			}
			if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_source", []string{"repo_root", "source_path"}, false)); err != nil {
				return fmt.Errorf("migrating to v3 (refs source index): %w", err)
			}
			if _, err := adapter.Exec(dialect.CreateIndexSQL("symbol_refs", "idx_refs_scope", []string{"repo_root", "source_scope"}, false)); err != nil {
				return fmt.Errorf("migrating to v3 (refs scope index): %w", err)
			}

			// Add type_relations table
			typeRelationsColumns := []db.ColumnDef{
				{Name: "id", Type: db.ColTypeAutoIncrement},
				{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
				{Name: "child_type", Type: db.ColTypeText, Nullable: false},
				{Name: "parent_type", Type: db.ColTypeText, Nullable: false},
				{Name: "relation", Type: db.ColTypeText, Nullable: false},
				{Name: "path", Type: db.ColTypeText, Nullable: false},
				{Name: "line", Type: db.ColTypeInteger, Nullable: false},
			}
			if _, err := adapter.Exec(dialect.CreateTableSQL("type_relations", typeRelationsColumns)); err != nil {
				return fmt.Errorf("migrating to v3 (type_relations table): %w", err)
			}

			// Create indexes on type_relations
			if _, err := adapter.Exec(dialect.CreateIndexSQL("type_relations", "idx_type_relations_unique", []string{"repo_root", "child_type", "parent_type", "path"}, true)); err != nil {
				// Ignore if exists
			}
			if _, err := adapter.Exec(dialect.CreateIndexSQL("type_relations", "idx_types_child", []string{"repo_root", "child_type"}, false)); err != nil {
				return fmt.Errorf("migrating to v3 (types child index): %w", err)
			}
			if _, err := adapter.Exec(dialect.CreateIndexSQL("type_relations", "idx_types_parent", []string{"repo_root", "parent_type"}, false)); err != nil {
				return fmt.Errorf("migrating to v3 (types parent index): %w", err)
			}
		}

		// Update schema version
		updateVersionSQL := fmt.Sprintf("UPDATE schema_version SET version = %s", dialect.Placeholder(1))
		if _, err := adapter.Exec(updateVersionSQL, schemaVersion); err != nil {
			return fmt.Errorf("updating schema version: %w", err)
		}
	}

	return nil
}
