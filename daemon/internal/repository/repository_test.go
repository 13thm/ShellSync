package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return db
}

func TestOpenAppliesPragmas(t *testing.T) {
	db := openTestDB(t)

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}

func TestMigrateCreatesAllTables(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	want := []string{
		"users", "devices", "tasks", "terminals", "terminal_logs",
		"todos", "settings", "sync_cursors", "schema_version",
	}
	for _, tbl := range want {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing: %v", tbl, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// only one migration recorded
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema_version rows = %d, want 1", n)
	}
}

func TestSeedDefaults(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := SeedDefaults(ctx, db); err != nil {
		t.Fatalf("first SeedDefaults: %v", err)
	}
	// second call must be a no-op (no duplicate user)
	if err := SeedDefaults(ctx, db); err != nil {
		t.Fatalf("second SeedDefaults: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("users count = %d, want 1", count)
	}

	var id string
	if err := db.QueryRow("SELECT id FROM users WHERE username='local'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != DefaultUserID {
		t.Fatalf("seeded id = %q, want %q", id, DefaultUserID)
	}
}
