package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenModernc(t *testing.T) {
	t.Run("opens in-memory database", func(t *testing.T) {
		cfg := Config{
			Driver:    DriverModernc,
			Path:      ":memory:",
			EnableWAL: false,
		}

		db, err := OpenModernc(cfg)
		if err != nil {
			t.Fatalf("OpenModernc() error = %v", err)
		}
		defer db.Close()

		// Verify we can execute queries
		_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}

		_, err = db.Exec("INSERT INTO test (name) VALUES (?)", "hello")
		if err != nil {
			t.Fatalf("Insert error = %v", err)
		}

		var name string
		err = db.QueryRow("SELECT name FROM test WHERE id = 1").Scan(&name)
		if err != nil {
			t.Fatalf("QueryRow() error = %v", err)
		}
		if name != "hello" {
			t.Errorf("got name = %q, want %q", name, "hello")
		}
	})

	t.Run("opens file database with WAL", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		cfg := Config{
			Driver:    DriverModernc,
			Path:      dbPath,
			EnableWAL: true,
		}

		db, err := OpenModernc(cfg)
		if err != nil {
			t.Fatalf("OpenModernc() error = %v", err)
		}
		defer db.Close()

		// Verify WAL mode is enabled
		var mode string
		err = db.QueryRow("PRAGMA journal_mode").Scan(&mode)
		if err != nil {
			t.Fatalf("PRAGMA journal_mode error = %v", err)
		}
		if mode != "wal" {
			t.Errorf("journal_mode = %q, want %q", mode, "wal")
		}

		// Verify file was created
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Error("database file was not created")
		}
	})

	t.Run("creates parent directory if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "subdir", "nested", "test.db")

		cfg := Config{
			Driver: DriverModernc,
			Path:   dbPath,
		}

		db, err := OpenModernc(cfg)
		if err != nil {
			t.Fatalf("OpenModernc() error = %v", err)
		}
		defer db.Close()

		// Verify directory was created
		dir := filepath.Dir(dbPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("parent directory was not created")
		}
	})
}

func TestModerncDB_Query(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	setupTestTable(t, db)

	t.Run("Query returns rows", func(t *testing.T) {
		rows, err := db.Query("SELECT id, name FROM test ORDER BY id")
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		defer rows.Close()

		var results []string
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			results = append(results, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows.Err() = %v", err)
		}

		if len(results) != 3 {
			t.Errorf("got %d rows, want 3", len(results))
		}
	})

	t.Run("QueryContext respects context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := db.QueryContext(ctx, "SELECT * FROM test")
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}

func TestModerncDB_Transaction(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, err := db.Exec("CREATE TABLE txtest (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE error = %v", err)
	}

	t.Run("commit persists changes", func(t *testing.T) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Begin() error = %v", err)
		}

		_, err = tx.Exec("INSERT INTO txtest (value) VALUES (?)", "committed")
		if err != nil {
			t.Fatalf("Insert error = %v", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		var count int
		db.QueryRow("SELECT COUNT(*) FROM txtest WHERE value = 'committed'").Scan(&count)
		if count != 1 {
			t.Errorf("got count = %d, want 1", count)
		}
	})

	t.Run("rollback discards changes", func(t *testing.T) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Begin() error = %v", err)
		}

		_, err = tx.Exec("INSERT INTO txtest (value) VALUES (?)", "rolled_back")
		if err != nil {
			t.Fatalf("Insert error = %v", err)
		}

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}

		var count int
		db.QueryRow("SELECT COUNT(*) FROM txtest WHERE value = 'rolled_back'").Scan(&count)
		if count != 0 {
			t.Errorf("got count = %d, want 0", count)
		}
	})

	t.Run("prepared statement works in transaction", func(t *testing.T) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Begin() error = %v", err)
		}
		defer tx.Rollback()

		stmt, err := tx.Prepare("INSERT INTO txtest (value) VALUES (?)")
		if err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}
		defer stmt.Close()

		for i := 0; i < 3; i++ {
			_, err := stmt.Exec("prepared")
			if err != nil {
				t.Fatalf("stmt.Exec() error = %v", err)
			}
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		var count int
		db.QueryRow("SELECT COUNT(*) FROM txtest WHERE value = 'prepared'").Scan(&count)
		if count != 3 {
			t.Errorf("got count = %d, want 3", count)
		}
	})
}

func TestOpen(t *testing.T) {
	t.Run("opens with default driver", func(t *testing.T) {
		cfg := DefaultConfig(":memory:")

		db, err := Open(cfg)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db.Close()

		if err := db.Ping(); err != nil {
			t.Errorf("Ping() error = %v", err)
		}
	})

	t.Run("errors on unsupported driver", func(t *testing.T) {
		cfg := Config{
			Driver: "invalid",
			Path:   ":memory:",
		}

		_, err := Open(cfg)
		if err == nil {
			t.Error("expected error for invalid driver")
		}
	})

	t.Run("ncruces driver returns not implemented", func(t *testing.T) {
		cfg := Config{
			Driver: DriverNcruces,
			Path:   ":memory:",
		}

		_, err := Open(cfg)
		if err == nil {
			t.Error("expected error for ncruces driver")
		}
	})
}

func TestWrapSQL(t *testing.T) {
	// Create a raw sql.DB using modernc driver
	cfg := DefaultConfig(":memory:")
	moderncDB, err := OpenModernc(cfg)
	if err != nil {
		t.Fatalf("OpenModernc() error = %v", err)
	}
	defer moderncDB.Close()

	// Get the underlying sql.DB
	sqlDB := moderncDB.Unwrap()

	// Wrap it
	wrapped := WrapSQL(sqlDB)

	// Verify the wrapped DB works
	_, err = wrapped.Exec("CREATE TABLE wrap_test (id INTEGER)")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	_, err = wrapped.Exec("INSERT INTO wrap_test (id) VALUES (1)")
	if err != nil {
		t.Fatalf("Insert error = %v", err)
	}

	var id int
	err = wrapped.QueryRow("SELECT id FROM wrap_test").Scan(&id)
	if err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if id != 1 {
		t.Errorf("got id = %d, want 1", id)
	}
}

// Helper functions

func openTestDB(t *testing.T) *ModerncDB {
	t.Helper()
	cfg := Config{
		Driver: DriverModernc,
		Path:   ":memory:",
	}
	db, err := OpenModernc(cfg)
	if err != nil {
		t.Fatalf("OpenModernc() error = %v", err)
	}
	return db
}

func setupTestTable(t *testing.T, db DB) {
	t.Helper()
	_, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE error = %v", err)
	}

	for _, name := range []string{"alice", "bob", "charlie"} {
		_, err := db.Exec("INSERT INTO test (name) VALUES (?)", name)
		if err != nil {
			t.Fatalf("INSERT error = %v", err)
		}
	}
}
