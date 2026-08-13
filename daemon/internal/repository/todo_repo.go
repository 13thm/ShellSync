package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const todoCols = "id, user_id, task_id, terminal_id, title, content, status, priority, sort_order, created_at, updated_at"

func scanTodo(sc func(...any) error) (Todo, error) {
	var t Todo
	var taskID, termID, content sql.NullString
	err := sc(&t.ID, &t.UserID, &taskID, &termID, &t.Title, &content, &t.Status, &t.Priority, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	t.TaskID = nzStr(taskID)
	t.TerminalID = nzStr(termID)
	t.Content = nzStr(content)
	return t, nil
}

// TodoRepo provides CRUD access to the todos table.
type TodoRepo struct{ db *sql.DB }

// NewTodoRepo creates a TodoRepo.
func NewTodoRepo(db *sql.DB) *TodoRepo { return &TodoRepo{db: db} }

// TodoFilter narrows a todo listing.
type TodoFilter struct {
	TaskID string
	Status string
}

// TodoCreate holds the fields needed to create a todo.
type TodoCreate struct {
	UserID     string
	TaskID     string // optional
	TerminalID string // optional
	Title      string
	Content    string
	Priority   int
	Status     string // defaults to "pending"
}

// TodoPatch describes a partial update.
type TodoPatch struct {
	Title      *string
	Content    *string
	Status     *string
	Priority   *int
	TaskID     *string // empty string unbinds (-> NULL)
	TerminalID *string
	SortOrder  *int
}

// List returns todos for a user, optionally filtered.
func (r *TodoRepo) List(ctx context.Context, userID string, f TodoFilter) ([]Todo, error) {
	q := "SELECT " + todoCols + " FROM todos WHERE user_id = ?"
	args := []any{userID}
	if f.TaskID != "" {
		q += " AND task_id = ?"
		args = append(args, f.TaskID)
	}
	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, f.Status)
	}
	q += " ORDER BY sort_order ASC, created_at ASC"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Todo
	for rows.Next() {
		t, err := scanTodo(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns a single todo by id.
func (r *TodoRepo) Get(ctx context.Context, id string) (Todo, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+todoCols+" FROM todos WHERE id = ?", id)
	return scanTodo(row.Scan)
}

// Create inserts a new todo and returns the stored row.
func (r *TodoRepo) Create(ctx context.Context, in TodoCreate) (Todo, error) {
	if in.Title == "" {
		return Todo{}, errors.New("todo: title required")
	}
	id := uuid.NewString()
	now := nowMs()
	status := in.Status
	if status == "" {
		status = "pending"
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO todos (id, user_id, task_id, terminal_id, title, content, status, priority, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, in.UserID, nullableStr(in.TaskID), nullableStr(in.TerminalID),
		in.Title, nullableStr(in.Content), status, in.Priority, now, now,
	)
	if err != nil {
		return Todo{}, err
	}
	return r.Get(ctx, id)
}

// Update applies a partial update and returns the stored row.
func (r *TodoRepo) Update(ctx context.Context, id string, p TodoPatch) (Todo, error) {
	var ub updateBuilder
	if p.Title != nil {
		ub.set("title", *p.Title)
	}
	if p.Content != nil {
		ub.set("content", nullableStr(*p.Content))
	}
	if p.Status != nil {
		ub.set("status", *p.Status)
	}
	if p.Priority != nil {
		ub.set("priority", *p.Priority)
	}
	if p.TaskID != nil {
		ub.set("task_id", nullableStr(*p.TaskID))
	}
	if p.TerminalID != nil {
		ub.set("terminal_id", nullableStr(*p.TerminalID))
	}
	if p.SortOrder != nil {
		ub.set("sort_order", *p.SortOrder)
	}
	if len(ub.sets) == 0 {
		return r.Get(ctx, id)
	}
	ub.set("updated_at", nowMs())

	q := "UPDATE todos SET " + strings.Join(ub.sets, ", ") + " WHERE id = ?"
	args := append(ub.args, id)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return Todo{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Todo{}, sql.ErrNoRows
	}
	return r.Get(ctx, id)
}

// Delete removes a todo.
func (r *TodoRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM todos WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListByTask returns todos bound to a task (any user).
func (r *TodoRepo) ListByTask(ctx context.Context, taskID string) ([]Todo, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+todoCols+" FROM todos WHERE task_id = ? ORDER BY sort_order ASC, created_at ASC", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Todo
	for rows.Next() {
		t, err := scanTodo(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListSince returns todos updated after since (for incremental sync).
func (r *TodoRepo) ListSince(ctx context.Context, userID string, since int64) ([]Todo, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+todoCols+" FROM todos WHERE user_id = ? AND updated_at > ? ORDER BY updated_at ASC",
		userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Todo
	for rows.Next() {
		t, err := scanTodo(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
