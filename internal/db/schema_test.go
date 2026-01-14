package db

import (
	"testing"
)

func TestSchemaBuilder_SubstitutePlaceholders(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		input   string
		want    string
	}{
		{
			name:    "SQLite no change",
			dialect: &SQLiteDialect{},
			input:   "SELECT * FROM t WHERE id = ? AND name = ?",
			want:    "SELECT * FROM t WHERE id = ? AND name = ?",
		},
		{
			name:    "Postgres substitution",
			dialect: &PostgresDialect{},
			input:   "SELECT * FROM t WHERE id = ? AND name = ?",
			want:    "SELECT * FROM t WHERE id = $1 AND name = $2",
		},
		{
			name:    "ClickHouse no change",
			dialect: &ClickHouseDialect{},
			input:   "SELECT * FROM t WHERE id = ? AND name = ?",
			want:    "SELECT * FROM t WHERE id = ? AND name = ?",
		},
		{
			name:    "Postgres complex query",
			dialect: &PostgresDialect{},
			input:   "INSERT INTO t (a, b, c) VALUES (?, ?, ?) ON CONFLICT (a) DO UPDATE SET b = ?, c = ?",
			want:    "INSERT INTO t (a, b, c) VALUES ($1, $2, $3) ON CONFLICT (a) DO UPDATE SET b = $4, c = $5",
		},
		{
			name:    "No placeholders",
			dialect: &PostgresDialect{},
			input:   "SELECT * FROM t",
			want:    "SELECT * FROM t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock DB for the schema builder (we won't execute anything)
			schema := NewSchemaBuilder(nil, tt.dialect)
			got := schema.SubstitutePlaceholders(tt.input)
			if got != tt.want {
				t.Errorf("SubstitutePlaceholders(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestQueryBuilder_SQL(t *testing.T) {
	schema := NewSchemaBuilder(nil, &SQLiteDialect{})

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{
			name: "simple select all",
			fn: func() string {
				return schema.Query("users").SQL()
			},
			want: "SELECT * FROM users",
		},
		{
			name: "select specific columns",
			fn: func() string {
				return schema.Query("users").Select("id", "name", "email").SQL()
			},
			want: "SELECT id, name, email FROM users",
		},
		{
			name: "with where clause",
			fn: func() string {
				return schema.Query("users").Select("id", "name").Where("active = ?").SQL()
			},
			want: "SELECT id, name FROM users WHERE active = ?",
		},
		{
			name: "with multiple where clauses",
			fn: func() string {
				return schema.Query("users").
					Select("id").
					Where("active = ?").
					Where("role = ?").
					SQL()
			},
			want: "SELECT id FROM users WHERE active = ? AND role = ?",
		},
		{
			name: "with order by",
			fn: func() string {
				return schema.Query("users").OrderBy("created_at DESC").SQL()
			},
			want: "SELECT * FROM users ORDER BY created_at DESC",
		},
		{
			name: "with limit",
			fn: func() string {
				return schema.Query("users").Limit(10).SQL()
			},
			want: "SELECT * FROM users LIMIT 10",
		},
		{
			name: "with offset",
			fn: func() string {
				return schema.Query("users").Limit(10).Offset(20).SQL()
			},
			want: "SELECT * FROM users LIMIT 10 OFFSET 20",
		},
		{
			name: "full query",
			fn: func() string {
				return schema.Query("users").
					Select("id", "name").
					Where("active = ?").
					OrderBy("name ASC").
					Limit(25).
					Offset(50).
					SQL()
			},
			want: "SELECT id, name FROM users WHERE active = ? ORDER BY name ASC LIMIT 25 OFFSET 50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn()
			if got != tt.want {
				t.Errorf("SQL() = %q, want %q", got, tt.want)
			}
		})
	}
}
