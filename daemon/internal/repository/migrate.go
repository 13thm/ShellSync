package repository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DefaultUserID is the stable id of the single local user created by
// SeedDefaults (MVP single-user mode). Other tables reference users(id).
const DefaultUserID = "00000000-0000-0000-0000-000000000001"

func nowMs() int64 { return time.Now().UnixMilli() }

// Migrate applies all pending SQL migrations in order. It is idempotent and
// safe to call on every startup.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return migrationNumber(names[i]) < migrationNumber(names[j])
	})

	for _, name := range names {
		num := migrationNumber(name)
		applied, err := versionApplied(ctx, db, num)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(ctx, db, name, num); err != nil {
			return err
		}
		slog.Info("migration applied", "file", name, "version", num)
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, name string, num int) error {
	data, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, string(data)); err != nil {
		return fmt.Errorf("exec %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
		num, nowMs(),
	); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	return tx.Commit()
}

func versionApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var v int
	err := db.QueryRowContext(ctx,
		"SELECT version FROM schema_version WHERE version = ?", version,
	).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// migrationNumber parses the leading numeric prefix of a migration filename,
// e.g. "0001_init.sql" -> 1.
func migrationNumber(name string) int {
	prefix := strings.SplitN(name, "_", 2)[0]
	n, _ := strconv.Atoi(prefix)
	return n
}

// SeedDefaults ensures a single local user exists (MVP single-user mode).
// Calling it more than once is a no-op.
func SeedDefaults(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	now := nowMs()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		DefaultUserID, "local", "Local User", now, now,
	); err != nil {
		return fmt.Errorf("seed default user: %w", err)
	}
	slog.Info("default user seeded", "id", DefaultUserID)
	return nil
}
