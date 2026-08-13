package repository

import (
	"context"
	"database/sql"
)

// SettingsRepo provides access to the settings KV table.
type SettingsRepo struct{ db *sql.DB }

// NewSettingsRepo creates a SettingsRepo.
func NewSettingsRepo(db *sql.DB) *SettingsRepo { return &SettingsRepo{db: db} }

// Get returns the value for a key. The bool is false when the key is absent.
func (r *SettingsRepo) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := r.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// GetAll returns all settings as a map.
func (r *SettingsRepo) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Set upserts a setting.
func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// Delete removes a setting. No error if the key is absent.
func (r *SettingsRepo) Delete(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", key)
	return err
}
