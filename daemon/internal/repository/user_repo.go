package repository

import (
	"context"
	"database/sql"
)

// UserRepo provides read access to the users table.
type UserRepo struct{ db *sql.DB }

// NewUserRepo creates a UserRepo.
func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

// Get returns a single user by id.
func (r *UserRepo) Get(ctx context.Context, id string) (User, error) {
	var u User
	var displayName sql.NullString
	err := r.db.QueryRowContext(ctx,
		"SELECT id, username, display_name, created_at, updated_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &displayName, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return User{}, err
	}
	u.DisplayName = nzStr(displayName)
	return u, nil
}

// GetDefault returns the single local user (MVP single-user mode).
func (r *UserRepo) GetDefault(ctx context.Context) (User, error) {
	return r.Get(ctx, DefaultUserID)
}
