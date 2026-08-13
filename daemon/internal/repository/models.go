package repository

import (
	"database/sql"
)

// Null/bool helpers ---------------------------------------------------------

// nzStr converts a sql.NullString to a plain string ("" when NULL).
func nzStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// nullableStr returns nil (SQL NULL) for an empty string, else the string.
// Used when writing nullable text/FK columns where "" is not a valid value.
func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ptrInt(v int) *int { return &v }

// updateBuilder builds a dynamic "SET col=?, ..." clause for PATCH updates.
type updateBuilder struct {
	sets []string
	args []any
}

func (b *updateBuilder) set(col string, val any) {
	b.sets = append(b.sets, col+" = ?")
	b.args = append(b.args, val)
}

// Domain models -------------------------------------------------------------

// User is a row in users (MVP: a single seeded local user).
type User struct {
	ID          string
	Username    string
	DisplayName string
	CreatedAt   int64
	UpdatedAt   int64
}

// Device is a paired client device.
type Device struct {
	ID           string
	UserID       string
	Name         string
	Platform     string
	SessionToken string
	LastSeenAt   int64 // 0 if NULL
	CreatedAt    int64
	Revoked      bool
}

// Task is a business task.
type Task struct {
	ID          string
	UserID      string
	Name        string
	Description string
	Status      string // pending|running|paused|done
	Color       string
	Archived    bool
	CreatedAt   int64
	UpdatedAt   int64
}

// Terminal is a live or historical terminal session row.
type Terminal struct {
	ID           string
	UserID       string
	TaskID       string // "" when NULL (unbound)
	Name         string
	ShellType    string
	Cwd          string
	Cols         int
	Rows         int
	Env          string
	Status       string // running|exited|crashed
	ExitCode     *int   // nil while running
	OsPID        *int   // nil if not running
	LastSeq      int64
	CreatedAt    int64
	LastActiveAt int64
	UpdatedAt    int64
}

// TerminalLog is one I/O chunk of a terminal.
type TerminalLog struct {
	ID         int64
	TerminalID string
	Seq        int64
	Direction  string // stdout|stderr|stdin|system
	ContentB64 string
	CreatedAt  int64
}

// Todo is a todo item, optionally linked to a task/terminal.
type Todo struct {
	ID         string
	UserID     string
	TaskID     string // "" when NULL
	TerminalID string // "" when NULL
	Title      string
	Content    string
	Status     string // pending|done
	Priority   int
	SortOrder  int
	CreatedAt  int64
	UpdatedAt  int64
}
