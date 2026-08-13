package http

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shellsync/daemon/internal/repository"
	"github.com/shellsync/daemon/internal/terminal"
)

// decodeJSON decodes the request body into v. An empty body is allowed.
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func queryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func queryInt64(r *http.Request, key string, def int64) int64 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// ---- tasks ---------------------------------------------------------------

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	archived := (*bool)(nil)
	if a := r.URL.Query().Get("archived"); a != "" {
		b := a == "1" || a == "true"
		archived = &b
	}
	filter := repository.TaskFilter{Status: r.URL.Query().Get("status"), Archived: archived}
	tasks, err := s.Svc.Tasks.List(r.Context(), filter)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, tasksToDTO(tasks))
}

type createTaskReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Status      string `json:"status"`
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var body createTaskReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.Svc.Tasks.Create(r.Context(), repository.TaskCreate{
		Name: body.Name, Description: body.Description, Color: body.Color, Status: body.Status,
	})
	if err != nil {
		fail(w, err)
		return
	}
	created(w, taskToDTO(t))
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Svc.Tasks.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, taskToDTO(t))
}

type updateTaskReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Color       *string `json:"color"`
	Archived    *bool   `json:"archived"`
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	var body updateTaskReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.Svc.Tasks.Update(r.Context(), chi.URLParam(r, "id"), repository.TaskPatch{
		Name: body.Name, Description: body.Description, Status: body.Status, Color: body.Color, Archived: body.Archived,
	})
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, taskToDTO(t))
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.Tasks.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"deleted": true})
}

// ---- todos ---------------------------------------------------------------

func (s *Server) listTodos(w http.ResponseWriter, r *http.Request) {
	filter := repository.TodoFilter{
		TaskID: r.URL.Query().Get("taskId"), Status: r.URL.Query().Get("status"),
	}
	todos, err := s.Svc.Todos.List(r.Context(), filter)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, todosToDTO(todos))
}

type createTodoReq struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	TaskID     string `json:"taskId"`
	TerminalID string `json:"terminalID"`
	Priority   int    `json:"priority"`
}

func (s *Server) createTodo(w http.ResponseWriter, r *http.Request) {
	var body createTodoReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.Svc.Todos.Create(r.Context(), repository.TodoCreate{
		Title: body.Title, Content: body.Content, TaskID: body.TaskID,
		TerminalID: body.TerminalID, Priority: body.Priority,
	})
	if err != nil {
		fail(w, err)
		return
	}
	created(w, todoToDTO(t))
}

func (s *Server) getTodo(w http.ResponseWriter, r *http.Request) {
	t, err := s.Svc.Todos.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, todoToDTO(t))
}

type updateTodoReq struct {
	Title      *string `json:"title"`
	Content    *string `json:"content"`
	Status     *string `json:"status"`
	Priority   *int    `json:"priority"`
	TaskID     *string `json:"taskId"`
	TerminalID *string `json:"terminalID"`
	SortOrder  *int    `json:"sortOrder"`
}

func (s *Server) updateTodo(w http.ResponseWriter, r *http.Request) {
	var body updateTodoReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.Svc.Todos.Update(r.Context(), chi.URLParam(r, "id"), repository.TodoPatch{
		Title: body.Title, Content: body.Content, Status: body.Status, Priority: body.Priority,
		TaskID: body.TaskID, TerminalID: body.TerminalID, SortOrder: body.SortOrder,
	})
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, todoToDTO(t))
}

func (s *Server) deleteTodo(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.Todos.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"deleted": true})
}

// ---- terminals -----------------------------------------------------------

func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request) {
	filter := repository.TerminalFilter{
		TaskID: r.URL.Query().Get("taskId"), Status: r.URL.Query().Get("status"),
	}
	terms, err := s.Svc.Terminals.List(r.Context(), filter)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, terminalsToDTO(terms))
}

type createTerminalReq struct {
	TaskID    string            `json:"taskId"`
	Name      string            `json:"name"`
	ShellType string            `json:"shellType"`
	Cwd       string            `json:"cwd"`
	Cols      int               `json:"cols"`
	Rows      int               `json:"rows"`
	Env       map[string]string `json:"env"`
}

func (s *Server) createTerminal(w http.ResponseWriter, r *http.Request) {
	var body createTerminalReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.Svc.Terminals.Create(r.Context(), terminal.CreateOpts{
		TaskID: body.TaskID, Name: body.Name, ShellType: body.ShellType,
		Cwd: body.Cwd, Cols: body.Cols, Rows: body.Rows, Env: body.Env,
	})
	if err != nil {
		failCode(w, codePTY, http.StatusInternalServerError, err.Error())
		return
	}
	t, _ := s.Svc.Terminals.Get(r.Context(), sess.ID())
	created(w, terminalToDTO(t))
}

func (s *Server) getTerminal(w http.ResponseWriter, r *http.Request) {
	t, err := s.Svc.Terminals.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, terminalToDTO(t))
}

type updateTerminalReq struct {
	Name   *string `json:"name"`
	TaskID *string `json:"taskId"`
}

func (s *Server) updateTerminal(w http.ResponseWriter, r *http.Request) {
	var body updateTerminalReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.Svc.Terminals.Update(r.Context(), chi.URLParam(r, "id"), repository.TerminalPatch{
		Name: body.Name, TaskID: body.TaskID,
	})
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, terminalToDTO(t))
}

type resizeReq struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

func (s *Server) resizeTerminal(w http.ResponseWriter, r *http.Request) {
	var body resizeReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	sess, alive := s.Svc.Terminals.Session(chi.URLParam(r, "id"))
	if !alive {
		failCode(w, codeConflict, http.StatusConflict, "terminal not running")
		return
	}
	if err := sess.Resize(body.Cols, body.Rows); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"resized": true})
}

func (s *Server) restartTerminal(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Svc.Terminals.Restart(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	t, _ := s.Svc.Terminals.Get(r.Context(), sess.ID())
	ok(w, terminalToDTO(t))
}

func (s *Server) deleteTerminal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Svc.Terminals.Stop(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"deleted": true})
}

func (s *Server) terminalLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fromSeq := queryInt64(r, "fromSeq", 1)
	limit := queryInt(r, "limit", 500)
	chunks, hasMore, err := s.Svc.Terminals.Logs(r.Context(), id, fromSeq, limit)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, LogsResponse{TerminalID: id, Chunks: logsToDTO(chunks), HasMore: hasMore})
}

func (s *Server) terminalLogsTail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit := queryInt(r, "limit", 500)
	chunks, err := s.Svc.Terminals.LogTail(r.Context(), id, limit)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, LogsResponse{TerminalID: id, Chunks: logsToDTO(chunks), HasMore: false})
}

// ---- pairing & devices ---------------------------------------------------

func (s *Server) pairInit(w http.ResponseWriter, r *http.Request) {
	res, err := s.Svc.Pair.Init(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, res)
}

type pairVerifyReq struct {
	PairingCode string `json:"pairingCode"`
	DeviceName  string `json:"deviceName"`
	Platform    string `json:"platform"`
}

func (s *Server) pairVerify(w http.ResponseWriter, r *http.Request) {
	var body pairVerifyReq
	if err := decodeJSON(r, &body); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.Svc.Pair.Verify(r.Context(), body.PairingCode, body.DeviceName, body.Platform)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, res)
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	devs, err := s.Svc.Devices.List(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, devicesToDTO(devs))
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.Svc.Devices.Revoke(r.Context(), chi.URLParam(r, "id")); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"revoked": true})
}

// ---- settings & system ---------------------------------------------------

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	kv, err := s.Svc.Settings.GetAll(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, kv)
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var kv map[string]string
	if err := decodeJSON(r, &kv); err != nil {
		failCode(w, codeValidation, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Svc.Settings.Patch(r.Context(), kv); err != nil {
		fail(w, err)
		return
	}
	ok(w, kv)
}

func (s *Server) listShells(w http.ResponseWriter, r *http.Request) {
	ok(w, detectShells())
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	ok(w, map[string]any{
		"ok": true, "version": s.Version,
		"uptime": int64(time.Since(s.StartedAt).Seconds()),
	})
}

func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	if s.Shutdown == nil {
		failCode(w, codeInternal, http.StatusInternalServerError, "shutdown not configured")
		return
	}
	ok(w, map[string]bool{"shuttingDown": true})
	go func() {
		// give the response time to flush before the server stops
		time.Sleep(100 * time.Millisecond)
		s.Shutdown()
	}()
}

// detectShells returns the shells discoverable on this platform.
func detectShells() []ShellDTO {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{"cmd", "powershell", "pwsh", "bash"}
	} else {
		candidates = []string{"bash", "zsh", "sh"}
	}
	out := make([]ShellDTO, 0, len(candidates))
	for _, c := range candidates {
		path, err := exec.LookPath(c)
		out = append(out, ShellDTO{Type: c, Path: path, Available: err == nil})
	}
	return out
}
