# Current Work Summary

Executing: Multi-Database Adapter Layer

**Branch:** `para/multi-database-adapter`
**Plan:** context/plans/2026-01-12-multi-database-adapter.md

## To-Do List

### Phase 1: SQL Dialect Abstraction
- [ ] Create `Dialect` interface in `internal/db/dialect.go`
- [ ] Implement SQLite dialect in `internal/db/dialect_sqlite.go`
- [ ] Create Postgres dialect stub in `internal/db/dialect_postgres.go`
- [ ] Create ClickHouse dialect stub in `internal/db/dialect_clickhouse.go`

### Phase 2: Update Config and Driver System
- [ ] Add `DatabaseType` enum to adapter.go
- [ ] Expand `Config` struct with DSN, Type, and connection options
- [ ] Update `Open()` to support database type selection

### Phase 3: Schema Builder
- [ ] Create schema builder in `internal/db/schema.go`
- [ ] Implement `CreateTable()`, `CreateIndex()`, `Upsert()` methods
- [ ] Add placeholder substitution for parameterized queries

### Phase 4: Update Symbols Package
- [ ] Change `Index.db` from `*sql.DB` to `db.DB`
- [ ] Update `OpenDB()` to use adapter
- [ ] Replace raw SQL with dialect-aware methods
- [ ] Remove SQLite-specific PRAGMA calls from schema.go

### Phase 5: Update Embedding Store
- [ ] Use dialect-aware upsert instead of `INSERT OR REPLACE`
- [ ] Use placeholder substitution for queries

### Phase 6: Vector Search Abstraction
- [ ] Create `VectorDB` interface in `internal/db/vector.go`
- [ ] Add `DistanceMetric` enum
- [ ] Update `ExtendedDB` to use generic vector interface

### Final
- [ ] Add unit tests for dialect abstraction
- [ ] Update architecture documentation
- [ ] Verify all existing tests pass

## Progress Notes

_Update this section as you complete items._

---
```json
{
  "active_context": [
    "context/plans/2026-01-12-multi-database-adapter.md"
  ],
  "completed_summaries": [],
  "execution_branch": "para/multi-database-adapter",
  "execution_started": "2026-01-12T14:30:00Z",
  "last_updated": "2026-01-12T14:30:00Z"
}
```
