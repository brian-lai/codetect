# Plan: Multi-Database Adapter Layer

## Objective

Upgrade the database adapter layer to support multiple database technologies (PostgreSQL, ClickHouse, etc.) without implementing the specific drivers yet. This involves:

1. Abstracting SQL dialect differences
2. Updating all code to use the adapter interface
3. Creating extension points for future database implementations

## Current State Analysis

### SQLite-Specific Syntax Found

| Location | Syntax | Postgres Equivalent | ClickHouse |
|----------|--------|---------------------|------------|
| `modernc.go:41` | `PRAGMA journal_mode=WAL` | N/A (config) | N/A |
| `schema.go:20` | `INTEGER PRIMARY KEY AUTOINCREMENT` | `SERIAL PRIMARY KEY` | `UInt64` |
| `schema.go:58` | `PRAGMA journal_mode=WAL` | N/A | N/A |
| `index.go:180` | `INSERT OR REPLACE` | `INSERT ... ON CONFLICT DO UPDATE` | `INSERT` (ReplacingMergeTree) |
| `store.go:74,96` | `INSERT OR REPLACE` | `INSERT ... ON CONFLICT DO UPDATE` | `INSERT` |

### Files Still Using Raw `*sql.DB`

| File | Usage | Needs Update |
|------|-------|--------------|
| `internal/search/symbols/schema.go` | `OpenDB()`, helpers | Yes |
| `internal/search/symbols/index.go` | `Index.db` field | Yes |
| `internal/embedding/store.go` | `NewEmbeddingStoreFromSQL()` | Keep for compat |

## Approach

### Phase 1: SQL Dialect Abstraction

Create a `Dialect` interface to handle database-specific SQL generation:

```go
// internal/db/dialect.go
type Dialect interface {
    // Schema generation
    CreateTableSQL(table string, columns []ColumnDef) string
    CreateIndexSQL(table, name string, columns []string) string

    // Upsert handling
    UpsertSQL(table string, columns []string, conflictKeys []string) string

    // Placeholder style: ? (SQLite/MySQL) vs $1 (Postgres)
    Placeholder(index int) string

    // Database-specific settings (WAL, etc.)
    InitSQL() []string

    // Data types
    AutoIncrementType() string  // INTEGER PRIMARY KEY AUTOINCREMENT vs SERIAL
    BlobType() string           // BLOB vs BYTEA
    TimestampType() string      // INTEGER vs TIMESTAMPTZ
}
```

Implement for each database:
- `internal/db/dialect_sqlite.go`
- `internal/db/dialect_postgres.go` (stub)
- `internal/db/dialect_clickhouse.go` (stub)

### Phase 2: Update Config and Driver System

Expand `Config` to support different database types:

```go
type DatabaseType string

const (
    DatabaseSQLite     DatabaseType = "sqlite"
    DatabasePostgres   DatabaseType = "postgres"
    DatabaseClickHouse DatabaseType = "clickhouse"
)

type Config struct {
    Type     DatabaseType
    Driver   Driver        // For SQLite variants

    // Connection
    Path     string        // SQLite file path
    DSN      string        // Postgres/ClickHouse connection string

    // Options
    EnableWAL          bool
    VectorDimensions   int
    MaxConnections     int
    ConnectionTimeout  time.Duration
}
```

### Phase 3: Schema Builder

Create a schema builder that uses the dialect:

```go
// internal/db/schema.go
type SchemaBuilder struct {
    dialect Dialect
    db      DB
}

func (s *SchemaBuilder) CreateTable(name string, columns []ColumnDef) error
func (s *SchemaBuilder) CreateIndex(table, name string, columns []string) error
func (s *SchemaBuilder) Upsert(table string, data map[string]any, conflictKeys []string) error
```

### Phase 4: Update Symbols Package

Migrate `internal/search/symbols/` to use the adapter:

1. Change `Index.db` from `*sql.DB` to `db.DB`
2. Update `OpenDB()` to use `db.Open()`
3. Replace raw SQL with schema builder calls
4. Remove `PRAGMA` calls (move to dialect init)

### Phase 5: Update Embedding Store

Ensure `EmbeddingStore` uses dialect-aware SQL:

1. Replace `INSERT OR REPLACE` with `Upsert()` helper
2. Use `Placeholder()` for parameterized queries
3. Abstract schema creation

### Phase 6: Vector Search Abstraction

Expand `ExtendedDB` to be database-agnostic:

```go
type VectorDB interface {
    DB

    // Vector operations (implementation varies by DB)
    CreateVectorIndex(table, column string, dimensions int, metric DistanceMetric) error
    InsertVector(table string, id int64, vector []float32) error
    SearchKNN(table, column string, query []float32, k int) ([]VectorResult, error)
}

type DistanceMetric string

const (
    MetricCosine    DistanceMetric = "cosine"
    MetricEuclidean DistanceMetric = "euclidean"
    MetricDotProduct DistanceMetric = "dot"
)
```

## Files to Create

| File | Purpose |
|------|---------|
| `internal/db/dialect.go` | Dialect interface definition |
| `internal/db/dialect_sqlite.go` | SQLite dialect implementation |
| `internal/db/dialect_postgres.go` | Postgres dialect (stub) |
| `internal/db/dialect_clickhouse.go` | ClickHouse dialect (stub) |
| `internal/db/schema.go` | Schema builder using dialect |
| `internal/db/vector.go` | VectorDB interface |

## Files to Modify

| File | Changes |
|------|---------|
| `internal/db/adapter.go` | Add DatabaseType, expand Config |
| `internal/db/open.go` | Support new database types |
| `internal/search/symbols/schema.go` | Use adapter + dialect |
| `internal/search/symbols/index.go` | Use db.DB interface |
| `internal/embedding/store.go` | Use dialect-aware upsert |
| `docs/architecture.md` | Document multi-DB support |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing SQLite usage | Keep `NewEmbeddingStoreFromSQL()` for backward compat |
| SQL dialect edge cases | Start with common subset, expand as needed |
| Performance differences between DBs | Document expected behavior differences |
| Transaction semantics vary | Abstract only common transaction patterns |
| Vector search syntax varies widely | Keep `VectorDB` interface simple, impl handles complexity |

## Success Criteria

- [ ] All code uses `db.DB` interface (no raw `*sql.DB` except in adapters)
- [ ] No SQLite-specific SQL outside of `dialect_sqlite.go`
- [ ] Symbols package uses adapter interface
- [ ] Dialect stubs exist for Postgres and ClickHouse
- [ ] Existing SQLite functionality unchanged
- [ ] All tests pass
- [ ] Architecture docs updated

## Out of Scope (Future Work)

- Implementing actual Postgres driver
- Implementing actual ClickHouse driver
- Data migration between database types
- Connection pooling optimization
- Read replicas / write splitting

## Review Checklist

- [ ] Dialect interface covers all SQL differences found in codebase
- [ ] Schema builder handles all table/index creation patterns
- [ ] Upsert abstraction handles symbols and embeddings tables
- [ ] Config supports both file-based (SQLite) and DSN-based (PG) connections
- [ ] Vector interface is generic enough for pgvector and ClickHouse

---

*Created: 2026-01-12*
*Status: Pending Review*
