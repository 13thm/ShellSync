package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

const deviceCols = "id, user_id, name, platform, session_token, last_seen_at, created_at, revoked"

func scanDevice(sc func(...any) error) (Device, error) {
	var d Device
	var lastSeen sql.NullInt64
	var revoked int
	err := sc(&d.ID, &d.UserID, &d.Name, &d.Platform, &d.SessionToken, &lastSeen, &d.CreatedAt, &revoked)
	if err != nil {
		return d, err
	}
	if lastSeen.Valid {
		d.LastSeenAt = lastSeen.Int64
	}
	d.Revoked = revoked != 0
	return d, nil
}

// DeviceRepo provides CRUD access to the devices table.
type DeviceRepo struct{ db *sql.DB }

// NewDeviceRepo creates a DeviceRepo.
func NewDeviceRepo(db *sql.DB) *DeviceRepo { return &DeviceRepo{db: db} }

// DeviceCreate holds the fields needed to register a paired device.
type DeviceCreate struct {
	UserID       string
	Name         string
	Platform     string
	SessionToken string
}

// Create inserts a new device and returns the stored row.
func (r *DeviceRepo) Create(ctx context.Context, in DeviceCreate) (Device, error) {
	id := uuid.NewString()
	now := nowMs()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO devices (id, user_id, name, platform, session_token, last_seen_at, created_at, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		id, in.UserID, in.Name, in.Platform, in.SessionToken, now, now)
	if err != nil {
		return Device{}, err
	}
	return r.Get(ctx, id)
}

// Get returns a single device by id.
func (r *DeviceRepo) Get(ctx context.Context, id string) (Device, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+deviceCols+" FROM devices WHERE id = ?", id)
	return scanDevice(row.Scan)
}

// GetByToken returns the non-revoked device matching a session token.
func (r *DeviceRepo) GetByToken(ctx context.Context, token string) (Device, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+deviceCols+" FROM devices WHERE session_token = ? AND revoked = 0", token)
	return scanDevice(row.Scan)
}

// ListByUser returns all devices for a user.
func (r *DeviceRepo) ListByUser(ctx context.Context, userID string) ([]Device, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+deviceCols+" FROM devices WHERE user_id = ? ORDER BY created_at ASC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// TouchLastSeen updates the device's last-seen timestamp to now.
func (r *DeviceRepo) TouchLastSeen(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE devices SET last_seen_at = ? WHERE id = ?", nowMs(), id)
	return err
}

// Revoke marks a device as revoked (its token will no longer authenticate).
func (r *DeviceRepo) Revoke(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "UPDATE devices SET revoked = 1 WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes a device record entirely.
func (r *DeviceRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM devices WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
