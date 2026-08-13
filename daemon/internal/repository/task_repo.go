package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const taskCols = "id, user_id, name, description, status, color, archived, created_at, updated_at"

func scanTask(sc func(...any) error) (Task, error) {
	var t Task
	var desc, color sql.NullString
	var archived int
	err := sc(&t.ID, &t.UserID, &t.Name, &desc, &t.Status, &color, &archived, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	t.Description = nzStr(desc)
	t.Color = nzStr(color)
	t.Archived = archived != 0
	return t, nil
}

// TaskRepo provides CRUD access to the tasks table.
type TaskRepo struct{ db *sql.DB }

// NewTaskRepo creates a TaskRepo.
func NewTaskRepo(db *sql.DB) *TaskRepo { return &TaskRepo{db: db} }

// TaskFilter narrows a task listing. Zero-value fields are ignored.
type TaskFilter struct {
	Status   string
	Archived *bool
}

// TaskCreate holds the fields needed to create a task.
type TaskCreate struct {
	UserID      string
	Name        string
	Description string
	Color       string
	Status      string // defaults to "pending" when empty
}

// TaskPatch describes a partial update. Non-nil fields are applied.
type TaskPatch struct {
	Name        *string
	Description *string // empty string clears the field (-> NULL)
	Status      *string
	Color       *string
	Archived    *bool
}

// List returns tasks for a user, optionally filtered, newest first.
func (r *TaskRepo) List(ctx context.Context, userID string, f TaskFilter) ([]Task, error) {
	q := "SELECT " + taskCols + " FROM tasks WHERE user_id = ?"
	args := []any{userID}
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Archived != nil {
		q += " AND archived = ?"
		args = append(args, boolToInt(*f.Archived))
	}
	q += " ORDER BY updated_at DESC"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns a single task by id.
func (r *TaskRepo) Get(ctx context.Context, id string) (Task, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+taskCols+" FROM tasks WHERE id = ?", id)
	return scanTask(row.Scan)
}

// Create inserts a new task and returns the stored row.
func (r *TaskRepo) Create(ctx context.Context, in TaskCreate) (Task, error) {
	if in.Name == "" {
		return Task{}, errors.New("task: name required")
	}
	id := uuid.NewString()
	now := nowMs()
	status := in.Status
	if status == "" {
		status = "pending"
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tasks (id, user_id, name, description, status, color, archived, created_at, updated_at)
		 VALUES (?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), 0, ?, ?)`,
		id, in.UserID, in.Name, in.Description, status, in.Color, now, now,
	)
	if err != nil {
		return Task{}, err
	}
	return r.Get(ctx, id)
}

// Update applies a partial update and returns the stored row.
func (r *TaskRepo) Update(ctx context.Context, id string, p TaskPatch) (Task, error) {
	var ub updateBuilder
	if p.Name != nil {
		ub.set("name", *p.Name)
	}
	if p.Description != nil {
		ub.set("description", nullableStr(*p.Description))
	}
	if p.Status != nil {
		ub.set("status", *p.Status)
	}
	if p.Color != nil {
		ub.set("color", nullableStr(*p.Color))
	}
	if p.Archived != nil {
		ub.set("archived", boolToInt(*p.Archived))
	}
	if len(ub.sets) == 0 {
		return r.Get(ctx, id)
	}
	ub.set("updated_at", nowMs())

	q := "UPDATE tasks SET " + strings.Join(ub.sets, ", ") + " WHERE id = ?"
	args := append(ub.args, id)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return Task{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Task{}, sql.ErrNoRows
	}
	return r.Get(ctx, id)
}

// Delete removes a task. Bound terminals are unbound (task_id -> NULL) via FK.
func (r *TaskRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListSince returns tasks updated after since (for incremental sync).
func (r *TaskRepo) ListSince(ctx context.Context, userID string, since int64) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+taskCols+" FROM tasks WHERE user_id = ? AND updated_at > ? ORDER BY updated_at ASC",
		userID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Count returns the number of tasks for a user.
func (r *TaskRepo) Count(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE user_id = ?", userID).Scan(&n)
	return n, err
}
