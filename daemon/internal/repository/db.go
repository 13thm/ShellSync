// Package repository is the SQLite data-access layer.
package repository

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers "sqlite")
)

// Open opens the SQLite database at path with the recommended PRAGMAs
// (WAL journal, NORMAL synchronous, foreign keys, busy timeout).
//
// For the MVP the pool is pinned to a single connection so that the
// per-connection PRAGMAs apply uniformly; this can be widened later for read
// concurrency (see design §4.4).
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", path, err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}
	return db, nil
}
