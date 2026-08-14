package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/pty"
	"github.com/shellsync/daemon/internal/repository"
)

// CreateOpts configures a new terminal session.
type CreateOpts struct {
	UserID    string
	TaskID    string
	Name      string
	ShellType string
	Cwd       string
	Cols      int
	Rows      int
	Env       map[string]string
}

// Manager owns all live terminal sessions and wires PTY, log store and
// persistence together.
type Manager struct {
	termRepo *repository.TerminalRepo
	logMgr   *logstore.Manager

	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager creates a Manager. It routes log-store flushes to the
// corresponding session's output subscribers.
func NewManager(termRepo *repository.TerminalRepo, logMgr *logstore.Manager) *Manager {
	m := &Manager{
		termRepo: termRepo,
		logMgr:   logMgr,
		sessions: map[string]*Session{},
	}
	logMgr.OnFlush = m.onFlushedChunk
	return m
}

// onFlushedChunk is the log-store-wide hook; it dispatches to the live session
// that owns the terminal (if any).
func (m *Manager) onFlushedChunk(c logstore.FlushedChunk) {
	m.mu.RLock()
	sess := m.sessions[c.TerminalID]
	m.mu.RUnlock()
	if sess != nil {
		sess.notifyOutput(c)
	}
}

func (m *Manager) register(id string, s *Session) {
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
}

func (m *Manager) unregister(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// Create spawns a new shell process, persists a "running" terminal row, seeds
// the log sequence and starts the read loop.
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*Session, error) {
	if opts.ShellType == "" {
		opts.ShellType = defaultShellName()
	}
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("%s %s", opts.ShellType, time.Now().Format("15:04:05"))
	}
	envJSON := ""
	if len(opts.Env) > 0 {
		b, err := json.Marshal(opts.Env)
		if err != nil {
			return nil, fmt.Errorf("marshal env: %w", err)
		}
		envJSON = string(b)
	}

	p, err := pty.Spawn(pty.SpawnOpts{
		Shell: opts.ShellType, Cwd: opts.Cwd,
		Cols: opts.Cols, Rows: opts.Rows, Env: opts.Env,
	})
	if err != nil {
		return nil, err
	}

	term, err := m.termRepo.Create(ctx, repository.TerminalCreate{
		UserID: opts.UserID, TaskID: opts.TaskID, Name: opts.Name,
		ShellType: opts.ShellType, Cwd: opts.Cwd,
		Cols: opts.Cols, Rows: opts.Rows, Env: envJSON, Status: "running",
	})
	if err != nil {
		_ = p.Close()
		return nil, err
	}

	// Seed the log sequence from any pre-existing history (best effort).
	_ = m.logMgr.Register(ctx, term.ID)

	sess := newSession(m, term.ID, p)
	m.register(term.ID, sess)
	sess.start()
	return sess, nil
}

// Get returns the live session for id (ok=false if not currently running).
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns terminal metadata rows from the database.
func (m *Manager) List(ctx context.Context, userID string, f repository.TerminalFilter) ([]repository.Terminal, error) {
	return m.termRepo.List(ctx, userID, f)
}

// Stop forcibly closes a live session. Its DB row is marked "exited".
func (m *Manager) Stop(id string) error {
	m.mu.RLock()
	sess := m.sessions[id]
	m.mu.RUnlock()
	if sess == nil {
		return nil
	}
	sess.requestStop()
	<-sess.done
	return nil
}

// Restart re-spawns a crashed/exited terminal using its stored configuration.
func (m *Manager) Restart(ctx context.Context, id string) (*Session, error) {
	if _, ok := m.Get(id); ok {
		return nil, fmt.Errorf("terminal %s is already running", id)
	}
	term, err := m.termRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var env map[string]string
	if term.Env != "" {
		if err := json.Unmarshal([]byte(term.Env), &env); err != nil {
			return nil, fmt.Errorf("parse env: %w", err)
		}
	}
	p, err := pty.Spawn(pty.SpawnOpts{
		Shell: term.ShellType, Cwd: term.Cwd,
		Cols: term.Cols, Rows: term.Rows, Env: env,
	})
	if err != nil {
		return nil, err
	}
	if err := m.termRepo.UpdateStatus(ctx, id, "running", nil); err != nil {
		_ = p.Close()
		return nil, err
	}
	_ = m.logMgr.Register(ctx, id) // continue sequence after existing logs

	sess := newSession(m, id, p)
	m.register(id, sess)
	sess.start()
	return sess, nil
}

// RecoverOnStartup marks any "running" terminals as "crashed" since their host
// processes died with the previous daemon. Call once at daemon start.
func (m *Manager) RecoverOnStartup(ctx context.Context) (int64, error) {
	return m.termRepo.MarkRunningAsCrashed(ctx)
}

// CloseAll stops every live session. Call on daemon shutdown.
func (m *Manager) CloseAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// defaultShellName returns the platform default shell name used when a caller
// does not specify one.
func defaultShellName() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "bash"
}
