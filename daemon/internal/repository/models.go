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
	ID           string `json:"id"`
	UserID       string `json:"-"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	SessionToken string `json:"-"`          // never serialized
	LastSeenAt   int64  `json:"lastSeenAt"` // 0 if NULL
	CreatedAt    int64  `json:"createdAt"`
	Revoked      bool   `json:"revoked"`
}

// Task is a business task.
// JSON tags match the wire DTO so event payloads are directly usable by clients.
type Task struct {
	ID          string `json:"id"`
	UserID      string `json:"-"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // pending|running|paused|done
	Color       string `json:"color"`
	Archived    bool   `json:"archived"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Terminal is a live or historical terminal session row.
type Terminal struct {
	ID           string `json:"id"`
	UserID       string `json:"-"`
	TaskID       string `json:"taskId"` // "" when NULL (unbound)
	Name         string `json:"name"`
	ShellType    string `json:"shellType"`
	Cwd          string `json:"cwd"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
	Env          string `json:"-"`
	Status       string `json:"status"`   // running|exited|crashed
	ExitCode     *int   `json:"exitCode"` // nil while running
	OsPID        *int   `json:"-"`        // nil if not running
	LastSeq      int64  `json:"lastSeq"`
	CreatedAt    int64  `json:"createdAt"`
	LastActiveAt int64  `json:"lastActiveAt"`
	UpdatedAt    int64  `json:"updatedAt"`
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
	ID         string `json:"id"`
	UserID     string `json:"-"`
	TaskID     string `json:"taskId"`     // "" when NULL
	TerminalID string `json:"terminalID"` // "" when NULL
	Title      string `json:"title"`
	Content    string `json:"content"`
	Status     string `json:"status"` // pending|done
	Priority   int    `json:"priority"`
	SortOrder  int    `json:"sortOrder"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}
