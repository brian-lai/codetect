# Plan: Add `codetect clean` Command

**Date:** 2026-02-08
**Status:** Draft (Reviewed)
**Type:** Feature Addition

---

## Objective

Add a `codetect clean` command that properly cleans index data for both SQLite and PostgreSQL database backends, providing a consistent way to start fresh regardless of database choice.

## Problem Statement

Currently, users cannot cleanly reset their codetect indexes:

**SQLite:**
- `rm -rf .codetect/` works but is not discoverable
- User must know about the `.codetect/` directory structure
- No validation or confirmation prompts

**PostgreSQL:**
- `rm -rf .codetect/` does NOT delete index data (only the local Merkle tree cache)
- No built-in command to truncate tables
- Users must manually connect to database and run SQL commands
- Risk of deleting wrong data or missing tables

**Consistency Issue:**
- Same operation (`codetect index --force`) behaves differently on SQLite vs PostgreSQL
- UPSERT logic can leave stale data when chunking strategy changes between versions
- No way to guarantee a completely clean slate

## Data Inventory

### Files in `.codetect/`
| File | Source | Purpose |
|------|--------|---------|
| `index.db` | v2 indexer (SQLite) | Embeddings, chunk locations, cache |
| `symbols.db` | v1 indexer (SQLite) | Symbol definitions, file metadata |
| `merkle-tree.json` | v2 indexer | Incremental change detection tree |

Both database files can coexist if user has run both v1 and v2 indexers.

### Database Tables

**V2 Indexer Tables (in `index.db` or PostgreSQL):**

| Table | SQLite Name | PostgreSQL Name | Scoped by `repo_root` |
|-------|-------------|-----------------|----------------------|
| Embeddings | `embeddings` | `embeddings_{DIM}` (e.g., `embeddings_768`) | Yes |
| Embedding cache | `embedding_cache` | `embedding_cache_{DIM}` | No (content-addressed) |
| Chunk locations | `chunk_locations` | `chunk_locations` | Yes |
| Repo config | _(not used)_ | `repo_embedding_configs` | Yes (PK) |

**V1 Indexer Tables (in `symbols.db` or PostgreSQL):**

| Table | Name | Scoped by `repo_root` |
|-------|------|----------------------|
| Symbols | `symbols` | Yes |
| Files | `files` | Yes |
| Schema version | `schema_version` | No |

**Key detail:** PostgreSQL embedding table names are dynamic (`embeddings_{DIM}`). The actual dimensions depend on which embedding models have been used. Must discover tables at runtime, not hardcode names.

### Table Discovery Strategy (PostgreSQL)

To find all embedding/cache tables for a given repo:
1. Query `repo_embedding_configs` for the repo's dimensions
2. OR query `information_schema.tables` for tables matching `embeddings_%` and `embedding_cache_%`

Option 2 is more robust (works even if `repo_embedding_configs` is missing).

## Approach

### 1. Add `clean` subcommand to `codetect-index`

**Location:** `cmd/codetect-index/main.go`

Add new command handler:
```go
case "clean":
    runClean(os.Args[2:])
```

### 2. Implement `runClean` function

**Responsibilities:**
- Parse flags: `--force`, `--dry-run`
- Detect database type via `config.LoadDatabaseConfigFromEnv()`
- Check if daemon is running and warn user
- Inventory what will be cleaned (tables, files)
- Prompt for confirmation (unless `--force`)
- Execute cleanup based on database type
- Report results

**Flags:**
- `--force, -f`: Skip confirmation prompt
- `--dry-run, -n`: Show what would be cleaned without doing it

### 3. Database-specific cleanup logic

**SQLite cleanup:**
```go
func cleanSQLite(projectPath string, dryRun bool) error {
    // Clean both v1 and v2 databases + merkle tree
    filesToDelete := []string{
        filepath.Join(projectPath, ".codetect", "index.db"),
        filepath.Join(projectPath, ".codetect", "symbols.db"),
        filepath.Join(projectPath, ".codetect", "merkle-tree.json"),
    }

    if dryRun {
        // Show file sizes and what would be deleted
        return showSQLiteInfo(filesToDelete)
    }

    for _, f := range filesToDelete {
        if _, err := os.Stat(f); err == nil {
            os.Remove(f)
        }
    }
    return nil
}
```

**PostgreSQL cleanup:**
```go
func cleanPostgres(dsn string, repoRoot string, dryRun bool) error {
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return err
    }
    defer db.Close()

    // 1. Discover all embedding/cache tables via information_schema
    tables := discoverTables(db)
    // Returns: ["embeddings_768", "embeddings_1024", "embedding_cache_768", ...]

    // 2. Add known fixed tables
    fixedTables := []string{
        "chunk_locations",
        "repo_embedding_configs",
        "symbols",
        "files",
    }
    allTables := append(fixedTables, tables...)

    if dryRun {
        return showPostgresInfo(db, repoRoot, allTables)
    }

    // 3. Delete within a transaction, scoped to repo_root
    tx, _ := db.Begin()
    for _, table := range allTables {
        // Use WHERE repo_root = $1 to only delete this repo's data
        tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE repo_root = $1", table), repoRoot)
        // Ignore errors for tables that don't exist
    }
    tx.Commit()

    // 4. Also delete local merkle tree if it exists
    os.Remove(filepath.Join(repoRoot, ".codetect", "merkle-tree.json"))

    return nil
}

func discoverTables(db *sql.DB) []string {
    rows, _ := db.Query(`
        SELECT table_name FROM information_schema.tables
        WHERE table_schema = 'public'
        AND (table_name LIKE 'embeddings_%' OR table_name LIKE 'embedding_cache_%')
    `)
    // collect and return table names
}
```

### 4. Daemon check

Before cleaning, check if the daemon is running and watching this project:

```go
func checkDaemon(projectPath string) {
    socketPath := fmt.Sprintf("/tmp/codetect-%d.sock", os.Getuid())
    if _, err := os.Stat(socketPath); err == nil {
        // Daemon is running - warn user
        warn("codetect daemon is running. Stop it first with: codetect daemon stop")
        // Don't block, just warn
    }
}
```

### 5. Add wrapper command in `scripts/codetect-wrapper.sh`

```bash
cmd_clean() {
    load_config

    echo -e "${CYAN}Cleaning index data...${NC}"
    "$BIN_DIR/codetect-index" clean "$@"
}
```

Add to main switch statement and help text.

### 6. Safety features

**Confirmation prompt (SQLite):**
```
WARNING: This will permanently delete all indexed data for this project.

  Project: /Users/user/my-project
  Database: SQLite

  Files to delete:
    .codetect/index.db      (45.3 MB)
    .codetect/symbols.db    (2.1 MB)
    .codetect/merkle-tree.json  (128 KB)

Continue? [y/N]: _
```

**Confirmation prompt (PostgreSQL):**
```
WARNING: This will permanently delete all indexed data for this project.

  Project: /Users/user/my-project
  Database: PostgreSQL (localhost:5432/codetect)
  Repo root: /Users/user/my-project

  Tables to clean (rows scoped to this repo):
    chunk_locations       12,543 rows
    embeddings_768        12,543 rows
    embedding_cache_768   8,201 rows
    repo_embedding_configs    1 row
    symbols                 0 rows

  Local files to delete:
    .codetect/merkle-tree.json  (128 KB)

Continue? [y/N]: _
```

**Dry-run:** Same output as confirmation prompt, ending with "Run without --dry-run to execute."

## Files to Modify

1. **`cmd/codetect-index/main.go`**
   - Add `runClean()` function with flag parsing
   - Add `cleanSQLite()` and `cleanPostgres()` functions
   - Add `discoverTables()` for PostgreSQL table discovery
   - Add `checkDaemon()` for daemon warning
   - Add `showSQLiteInfo()` / `showPostgresInfo()` for dry-run output
   - Add to command switch and `printUsage()`

2. **`scripts/codetect-wrapper.sh`**
   - Add `cmd_clean()` function
   - Add `clean` case to main switch statement
   - Add to `cmd_help()` output

## Success Criteria

- [ ] `codetect clean` deletes both `index.db` and `symbols.db` for SQLite
- [ ] `codetect clean` deletes `merkle-tree.json`
- [ ] `codetect clean` discovers and cleans all dimension-specific PostgreSQL tables
- [ ] `codetect clean` scopes PostgreSQL deletes to `WHERE repo_root = ?`
- [ ] `--dry-run` shows what would be cleaned without executing
- [ ] Confirmation prompt prevents accidental deletions
- [ ] `--force` flag skips confirmation
- [ ] Daemon warning shown if daemon is running
- [ ] `codetect index` works correctly after `codetect clean` (full clean slate)
- [ ] Help text documents all flags and usage

## Risks & Mitigations

**Risk:** PostgreSQL cleanup affects other repos in shared database
- **Mitigation:** Always use `WHERE repo_root = ?` clause, never `TRUNCATE` or `DROP`

**Risk:** Cleanup leaves database in inconsistent state
- **Mitigation:** Use transactions for PostgreSQL; for SQLite, delete entire files (atomic)

**Risk:** User runs clean while daemon/indexing is in progress
- **Mitigation:** Check daemon socket and warn user before proceeding

**Risk:** Dynamic table discovery misses tables
- **Mitigation:** Query `information_schema` with LIKE patterns + include known fixed table names

## Scope Decisions

| Decision | Answer | Rationale |
|----------|--------|-----------|
| Clean `.codetectignore`? | No | Config, not index data |
| Remove project from registry? | No | Use `registry remove` explicitly |
| Selective cleanup (embeddings only)? | No | Start simple, add later if needed |
| Backup before cleaning? | No | Suggest manually; SQLite is just files |
| `--all` multi-project flag? | No | Defer; adds complexity, low priority |
| Update registry stats after clean? | No | Stats refresh on next `codetect index` |

## Implementation Order

1. Add `runClean()` with flag parsing and daemon check
2. Implement SQLite cleanup (delete files)
3. Implement PostgreSQL cleanup (table discovery + scoped deletes)
4. Add dry-run and confirmation prompt
5. Add wrapper script command + help text
6. Manual testing on SQLite and PostgreSQL
7. Commit and release

## Related Issues

- `codetect index --force` can leave stale chunks when chunking strategy changes
- No clean way to reset PostgreSQL-backed indexes
- Users don't know about `.codetect/` directory structure
