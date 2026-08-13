package repository

import (
	"context"
	"database/sql"
	"testing"
)

// newMigratedDB returns a freshly migrated database with the default user
// seeded. It reuses openTestDB (which registers a Close cleanup).
func newMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := SeedDefaults(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }
