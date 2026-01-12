package db

import (
	"context"
	"fmt"
	"strings"
)

// SchemaBuilder provides dialect-aware SQL generation and execution for schema operations.
type SchemaBuilder struct {
	db      DB
	dialect Dialect
}

// NewSchemaBuilder creates a new schema builder for the given database and dialect.
func NewSchemaBuilder(db DB, dialect Dialect) *SchemaBuilder {
	return &SchemaBuilder{
		db:      db,
		dialect: dialect,
	}
}

// NewSchemaBuilderFromConfig creates a schema builder using the dialect from config.
func NewSchemaBuilderFromConfig(db DB, cfg Config) *SchemaBuilder {
	return NewSchemaBuilder(db, cfg.Dialect())
}

// Dialect returns the underlying SQL dialect.
func (s *SchemaBuilder) Dialect() Dialect {
	return s.dialect
}

// CreateTable creates a table if it doesn't exist.
func (s *SchemaBuilder) CreateTable(ctx context.Context, table string, columns []ColumnDef) error {
	sql := s.dialect.CreateTableSQL(table, columns)
	_, err := s.db.ExecContext(ctx, sql)
	return err
}

// CreateIndex creates an index if it doesn't exist.
func (s *SchemaBuilder) CreateIndex(ctx context.Context, table, indexName string, columns []string, unique bool) error {
	sql := s.dialect.CreateIndexSQL(table, indexName, columns, unique)
	_, err := s.db.ExecContext(ctx, sql)
	return err
}

// Upsert performs an insert-or-update operation.
// columns: all columns to insert
// conflictColumns: columns that define uniqueness (for ON CONFLICT)
// updateColumns: columns to update on conflict (if nil, updates all non-conflict columns)
// values: values for all columns in order
func (s *SchemaBuilder) Upsert(ctx context.Context, table string, columns []string, conflictColumns []string, updateColumns []string, values ...any) (Result, error) {
	sql := s.dialect.UpsertSQL(table, columns, conflictColumns, updateColumns)
	return s.db.ExecContext(ctx, sql, values...)
}

// UpsertBatch performs batch upsert operations in a transaction.
func (s *SchemaBuilder) UpsertBatch(ctx context.Context, table string, columns []string, conflictColumns []string, updateColumns []string, rows [][]any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	sql := s.dialect.UpsertSQL(table, columns, conflictColumns, updateColumns)
	stmt, err := tx.Prepare(sql)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for i, row := range rows {
		if len(row) != len(columns) {
			return fmt.Errorf("row %d: expected %d values, got %d", i, len(columns), len(row))
		}
		if _, err := stmt.Exec(row...); err != nil {
			return fmt.Errorf("row %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// RunInitStatements executes dialect-specific initialization statements.
// For SQLite, this includes PRAGMA statements for WAL mode and foreign keys.
func (s *SchemaBuilder) RunInitStatements(ctx context.Context) error {
	for _, stmt := range s.dialect.InitStatements() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init statement %q: %w", stmt, err)
		}
	}
	return nil
}

// QueryBuilder provides a fluent interface for building and executing queries.
type QueryBuilder struct {
	schema *SchemaBuilder
	table  string
	cols   []string
	where  []string
	args   []any
	order  string
	limit  int
	offset int
}

// Query starts building a SELECT query for the given table.
func (s *SchemaBuilder) Query(table string) *QueryBuilder {
	return &QueryBuilder{
		schema: s,
		table:  table,
	}
}

// Select specifies the columns to select.
func (q *QueryBuilder) Select(cols ...string) *QueryBuilder {
	q.cols = cols
	return q
}

// Where adds a WHERE condition. Use dialect.Placeholder() for parameters.
// Example: Where("id = ?", 1) for SQLite, Where("id = $1", 1) for Postgres
func (q *QueryBuilder) Where(condition string, args ...any) *QueryBuilder {
	q.where = append(q.where, condition)
	q.args = append(q.args, args...)
	return q
}

// OrderBy sets the ORDER BY clause.
func (q *QueryBuilder) OrderBy(order string) *QueryBuilder {
	q.order = order
	return q
}

// Limit sets the LIMIT clause.
func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limit = n
	return q
}

// Offset sets the OFFSET clause.
func (q *QueryBuilder) Offset(n int) *QueryBuilder {
	q.offset = n
	return q
}

// SQL returns the generated SQL query string.
func (q *QueryBuilder) SQL() string {
	cols := "*"
	if len(q.cols) > 0 {
		cols = strings.Join(q.cols, ", ")
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", cols, q.table)

	if len(q.where) > 0 {
		sql += " WHERE " + strings.Join(q.where, " AND ")
	}

	if q.order != "" {
		sql += " ORDER BY " + q.order
	}

	if q.limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", q.limit)
	}

	if q.offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", q.offset)
	}

	return sql
}

// Exec executes the query and returns rows.
func (q *QueryBuilder) Exec(ctx context.Context) (Rows, error) {
	return q.schema.db.QueryContext(ctx, q.SQL(), q.args...)
}

// ExecRow executes the query expecting a single row.
func (q *QueryBuilder) ExecRow(ctx context.Context) Row {
	return q.schema.db.QueryRowContext(ctx, q.SQL(), q.args...)
}

// SubstitutePlaceholders converts ? placeholders to the dialect's format.
// This is useful for migrating raw SQL queries to be dialect-aware.
// Example: "SELECT * FROM t WHERE id = ? AND name = ?" becomes
// "SELECT * FROM t WHERE id = $1 AND name = $2" for PostgreSQL.
func (s *SchemaBuilder) SubstitutePlaceholders(sql string) string {
	if s.dialect.Name() == "sqlite" || s.dialect.Name() == "clickhouse" {
		// SQLite and ClickHouse use ? - no substitution needed
		return sql
	}

	// For dialects that use numbered placeholders ($1, $2, etc.)
	var result strings.Builder
	idx := 1
	for _, ch := range sql {
		if ch == '?' {
			result.WriteString(s.dialect.Placeholder(idx))
			idx++
		} else {
			result.WriteRune(ch)
		}
	}
	return result.String()
}
