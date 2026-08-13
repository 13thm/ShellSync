package repository

import (
	"context"
	"database/sql"
)

const logCols = "id, terminal_id, seq, direction, content_b64, created_at"

func scanLog(sc func(...any) error) (TerminalLog, error) {
	var l TerminalLog
	err := sc(&l.ID, &l.TerminalID, &l.Seq, &l.Direction, &l.ContentB64, &l.CreatedAt)
	return l, err
}

// LogRepo provides access to the terminal_logs table (chunked terminal I/O).
type LogRepo struct{ db *sql.DB }

// NewLogRepo creates a LogRepo.
func NewLogRepo(db *sql.DB) *LogRepo { return &LogRepo{db: db} }

// AppendChunk writes one I/O chunk. seq must be unique per terminal.
func (r *LogRepo) AppendChunk(ctx context.Context, terminalID string, seq int64, direction, contentB64 string, createdAt int64) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO terminal_logs (terminal_id, seq, direction, content_b64, created_at) VALUES (?, ?, ?, ?, ?)",
		terminalID, seq, direction, contentB64, createdAt)
	return err
}

// ReadRange returns up to limit chunks with seq >= fromSeq, in ascending order,
// plus a hasMore flag (true if more rows exist beyond the returned page).
func (r *LogRepo) ReadRange(ctx context.Context, terminalID string, fromSeq int64, limit int) ([]TerminalLog, bool, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+logCols+" FROM terminal_logs WHERE terminal_id = ? AND seq >= ? ORDER BY seq ASC LIMIT ?",
		terminalID, fromSeq, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	out := make([]TerminalLog, 0, limit)
	for rows.Next() {
		l, err := scanLog(rows.Scan)
		if err != nil {
			return nil, false, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// Tail returns the last `limit` chunks in ascending (oldest-first) order.
func (r *LogRepo) Tail(ctx context.Context, terminalID string, limit int) ([]TerminalLog, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+logCols+" FROM terminal_logs WHERE terminal_id = ? ORDER BY seq DESC LIMIT ?",
		terminalID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tmp []TerminalLog
	for rows.Next() {
		l, err := scanLog(rows.Scan)
		if err != nil {
			return nil, err
		}
		tmp = append(tmp, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// reverse to ascending order
	out := make([]TerminalLog, len(tmp))
	for i, l := range tmp {
		out[len(tmp)-1-i] = l
	}
	return out, nil
}

// MaxSeq returns the highest seq for a terminal (0 if none).
func (r *LogRepo) MaxSeq(ctx context.Context, terminalID string) (int64, error) {
	var seq int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) FROM terminal_logs WHERE terminal_id = ?", terminalID).Scan(&seq)
	return seq, err
}

// DeleteByTerminal removes all chunks for a terminal.
func (r *LogRepo) DeleteByTerminal(ctx context.Context, terminalID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, "DELETE FROM terminal_logs WHERE terminal_id = ?", terminalID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListBefore returns chunks older than cutoffMs (created_at < cutoff), oldest first.
// Used by the logstore cold-archiver.
func (r *LogRepo) ListBefore(ctx context.Context, terminalID string, cutoffMs int64) ([]TerminalLog, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+logCols+" FROM terminal_logs WHERE terminal_id = ? AND created_at < ? ORDER BY seq ASC",
		terminalID, cutoffMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TerminalLog
	for rows.Next() {
		l, err := scanLog(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteBefore removes chunks older than cutoffMs (created_at < cutoff).
func (r *LogRepo) DeleteBefore(ctx context.Context, terminalID string, cutoffMs int64) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"DELETE FROM terminal_logs WHERE terminal_id = ? AND created_at < ?", terminalID, cutoffMs)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
