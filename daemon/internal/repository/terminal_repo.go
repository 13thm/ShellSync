package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const terminalCols = "id, user_id, task_id, name, shell_type, cwd, cols, rows, env, status, exit_code, os_pid, last_seq, created_at, last_active_at, updated_at"

func scanTerminal(sc func(...any) error) (Terminal, error) {
	var t Terminal
	var taskID, cwd, env sql.NullString
	var exitCode, osPID sql.NullInt64
	err := sc(
		&t.ID, &t.UserID, &taskID, &t.Name, &t.ShellType, &cwd,
		&t.Cols, &t.Rows, &env, &t.Status, &exitCode, &osPID,
		&t.LastSeq, &t.CreatedAt, &t.LastActiveAt, &t.UpdatedAt,
	)
	if err != nil {
		return t, err
	}
	t.TaskID = nzStr(taskID)
	t.Cwd = nzStr(cwd)
	t.Env = nzStr(env)
	if exitCode.Valid {
		t.ExitCode = ptrInt(int(exitCode.Int64))
	}
	if osPID.Valid {
		t.OsPID = ptrInt(int(osPID.Int64))
	}
	return t, nil
}

// TerminalRepo provides CRUD access to the terminals table.
type TerminalRepo struct{ db *sql.DB }

// NewTerminalRepo creates a TerminalRepo.
func NewTerminalRepo(db *sql.DB) *TerminalRepo { return &TerminalRepo{db: db} }

// TerminalFilter narrows a terminal listing.
type TerminalFilter struct {
	TaskID string
	Status string
}

// TerminalCreate holds the fields needed to create a terminal row.
type TerminalCreate struct {
	UserID    string
	TaskID    string // optional
	Name      string
	ShellType string
	Cwd       string
	Cols      int
	Rows      int
	Env       string
	Status    string // defaults to "running"
}

// TerminalPatch describes a partial update.
type TerminalPatch struct {
	Name   *string
	TaskID *string // empty string unbinds (-> NULL)
}

// List returns terminals for a user, optionally filtered.
func (r *TerminalRepo) List(ctx context.Context, userID string, f TerminalFilter) ([]Terminal, error) {
	q := "SELECT " + terminalCols + " FROM terminals WHERE user_id = ?"
	args := []any{userID}
	if f.TaskID != "" {
		q += " AND task_id = ?"
		args = append(args, f.TaskID)
	}
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, f.Status)
	}
	q += " ORDER BY last_active_at DESC"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Terminal
	for rows.Next() {
		t, err := scanTerminal(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns a single terminal by id.
func (r *TerminalRepo) Get(ctx context.Context, id string) (Terminal, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+terminalCols+" FROM terminals WHERE id = ?", id)
	return scanTerminal(row.Scan)
}

// Create inserts a new terminal row and returns the stored row.
func (r *TerminalRepo) Create(ctx context.Context, in TerminalCreate) (Terminal, error) {
	if in.ShellType == "" {
		return Terminal{}, errors.New("terminal: shell_type required")
	}
	id := uuid.NewString()
	now := nowMs()
	status := in.Status
	if status == "" {
		status = "running"
	}
	cols := in.Cols
	if cols == 0 {
		cols = 80
	}
	rows := in.Rows
	if rows == 0 {
		rows = 24
	}
	name := in.Name
	if name == "" {
		name = "Terminal"
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO terminals
		 (id, user_id, task_id, name, shell_type, cwd, cols, rows, env, status, exit_code, os_pid, last_seq, created_at, last_active_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), ?, NULL, NULL, 0, ?, ?, ?)`,
		id, in.UserID, nullableStr(in.TaskID), name, in.ShellType, in.Cwd,
		cols, rows, in.Env, status, now, now, now,
	)
	if err != nil {
		return Terminal{}, err
	}
	return r.Get(ctx, id)
}

// Update applies a partial update and returns the stored row.
func (r *TerminalRepo) Update(ctx context.Context, id string, p TerminalPatch) (Terminal, error) {
	var ub updateBuilder
	if p.Name != nil {
		ub.set("name", *p.Name)
	}
	if p.TaskID != nil {
		ub.set("task_id", nullableStr(*p.TaskID))
	}
	if len(ub.sets) == 0 {
		return r.Get(ctx, id)
	}
	ub.set("updated_at", nowMs())

	q := "UPDATE terminals SET " + strings.Join(ub.sets, ", ") + " WHERE id = ?"
	args := append(ub.args, id)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return Terminal{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Terminal{}, sql.ErrNoRows
	}
	return r.Get(ctx, id)
}

// UpdateStatus sets the runtime status and optional exit code.
func (r *TerminalRepo) UpdateStatus(ctx context.Context, id, status string, exitCode *int) error {
	var ecArg any
	if exitCode != nil {
		ecArg = *exitCode
	}
	res, err := r.db.ExecContext(ctx,
		"UPDATE terminals SET status = ?, exit_code = ?, updated_at = ? WHERE id = ?",
		status, ecArg, nowMs(), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetLastSeq records the highest written log seq and refreshes activity time.
func (r *TerminalRepo) SetLastSeq(ctx context.Context, id string, seq int64) error {
	now := nowMs()
	_, err := r.db.ExecContext(ctx,
		"UPDATE terminals SET last_seq = ?, last_active_at = ?, updated_at = ? WHERE id = ?",
		seq, now, now, id,
	)
	return err
}

// Delete removes a terminal. Its logs cascade-delete via FK.
func (r *TerminalRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM terminals WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListByTask returns terminals bound to a task.
func (r *TerminalRepo) ListByTask(ctx context.Context, taskID string) ([]Terminal, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+terminalCols+" FROM terminals WHERE task_id = ? ORDER BY created_at ASC", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Terminal
	for rows.Next() {
		t, err := scanTerminal(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListSince returns terminals updated after since (for incremental sync).
func (r *TerminalRepo) ListSince(ctx context.Context, userID string, since int64) ([]Terminal, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+terminalCols+" FROM terminals WHERE user_id = ? AND updated_at > ? ORDER BY updated_at ASC",
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Terminal
	for rows.Next() {
		t, err := scanTerminal(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkRunningAsCrashed sets all "running" terminals to "crashed". Called on
// daemon startup to reflect that their host processes no longer exist.
func (r *TerminalRepo) MarkRunningAsCrashed(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"UPDATE terminals SET status = 'crashed', updated_at = ? WHERE status = 'running'", nowMs())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
