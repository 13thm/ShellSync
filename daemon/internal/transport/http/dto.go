package http

import (
	"github.com/shellsync/daemon/internal/repository"
)

// TaskDTO is the REST representation of a task.
type TaskDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Color       string `json:"color"`
	Archived    bool   `json:"archived"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

func taskToDTO(t repository.Task) TaskDTO {
	return TaskDTO{
		ID: t.ID, Name: t.Name, Description: t.Description, Status: t.Status,
		Color: t.Color, Archived: t.Archived, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func tasksToDTO(ts []repository.Task) []TaskDTO {
	out := make([]TaskDTO, len(ts))
	for i, t := range ts {
		out[i] = taskToDTO(t)
	}
	return out
}

// TerminalDTO is the REST representation of a terminal.
type TerminalDTO struct {
	ID           string `json:"id"`
	TaskID       string `json:"taskId"`
	Name         string `json:"name"`
	ShellType    string `json:"shellType"`
	Cwd          string `json:"cwd"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
	Status       string `json:"status"`
	ExitCode     *int   `json:"exitCode"`
	LastSeq      int64  `json:"lastSeq"`
	CreatedAt    int64  `json:"createdAt"`
	LastActiveAt int64  `json:"lastActiveAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

func terminalToDTO(t repository.Terminal) TerminalDTO {
	return TerminalDTO{
		ID: t.ID, TaskID: t.TaskID, Name: t.Name, ShellType: t.ShellType, Cwd: t.Cwd,
		Cols: t.Cols, Rows: t.Rows, Status: t.Status, ExitCode: t.ExitCode, LastSeq: t.LastSeq,
		CreatedAt: t.CreatedAt, LastActiveAt: t.LastActiveAt, UpdatedAt: t.UpdatedAt,
	}
}

func terminalsToDTO(ts []repository.Terminal) []TerminalDTO {
	out := make([]TerminalDTO, len(ts))
	for i, t := range ts {
		out[i] = terminalToDTO(t)
	}
	return out
}

// LogChunkDTO is one terminal history chunk.
type LogChunkDTO struct {
	Seq        int64  `json:"seq"`
	Direction  string `json:"direction"`
	ContentB64 string `json:"contentB64"`
	CreatedAt  int64  `json:"createdAt"`
}

func logsToDTO(ls []repository.TerminalLog) []LogChunkDTO {
	out := make([]LogChunkDTO, len(ls))
	for i, l := range ls {
		out[i] = LogChunkDTO{Seq: l.Seq, Direction: l.Direction, ContentB64: l.ContentB64, CreatedAt: l.CreatedAt}
	}
	return out
}

// LogsResponse wraps a page of chunks with a hasMore flag.
type LogsResponse struct {
	TerminalID string        `json:"terminalId"`
	Chunks     []LogChunkDTO `json:"chunks"`
	HasMore    bool          `json:"hasMore"`
}

// TodoDTO is the REST representation of a todo.
type TodoDTO struct {
	ID         string `json:"id"`
	TaskID     string `json:"taskId"`
	TerminalID string `json:"terminalID"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	Priority   int    `json:"priority"`
	SortOrder  int    `json:"sortOrder"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func todoToDTO(t repository.Todo) TodoDTO {
	return TodoDTO{
		ID: t.ID, TaskID: t.TaskID, TerminalID: t.TerminalID, Title: t.Title, Content: t.Content,
		Status: t.Status, Priority: t.Priority, SortOrder: t.SortOrder,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func todosToDTO(ts []repository.Todo) []TodoDTO {
	out := make([]TodoDTO, len(ts))
	for i, t := range ts {
		out[i] = todoToDTO(t)
	}
	return out
}

// DeviceDTO is the REST representation of a paired device.
type DeviceDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	LastSeenAt int64  `json:"lastSeenAt"`
	CreatedAt  int64  `json:"createdAt"`
	Revoked    bool   `json:"revoked"`
}

func devicesToDTO(ds []repository.Device) []DeviceDTO {
	out := make([]DeviceDTO, len(ds))
	for i, d := range ds {
		out[i] = DeviceDTO{
			ID: d.ID, Name: d.Name, Platform: d.Platform,
			LastSeenAt: d.LastSeenAt, CreatedAt: d.CreatedAt, Revoked: d.Revoked,
		}
	}
	return out
}

// ShellDTO describes a discoverable shell.
type ShellDTO struct {
	Type      string `json:"type"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
}
