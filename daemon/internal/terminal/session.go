package terminal

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/pty"
)

// OutputHandler is called (from an aggregator goroutine) when a new output
// chunk is persisted for this terminal.
type OutputHandler func(logstore.FlushedChunk)

// StatusHandler is called once when the session's process exits.
type StatusHandler func(status string, exitCode *int)

// Session is one live terminal attached to a running shell process.
type Session struct {
	id  string
	pty pty.PTY
	mgr *Manager

	finalizeOnce sync.Once
	done         chan struct{} // closed when finalize completes
	wg           sync.WaitGroup

	mu         sync.RWMutex
	forceStop  bool
	exited     bool
	status     string
	exitCode   *int
	nextSubID  int64
	outputSubs map[int64]OutputHandler
	statusSubs map[int64]StatusHandler
}

func newSession(m *Manager, id string, p pty.PTY) *Session {
	return &Session{
		id:         id,
		pty:        p,
		mgr:        m,
		done:       make(chan struct{}),
		outputSubs: map[int64]OutputHandler{},
		statusSubs: map[int64]StatusHandler{},
	}
}

// ID returns the terminal id.
func (s *Session) ID() string { return s.id }

// PID returns the child process id (0 if unavailable).
func (s *Session) PID() int { return s.pty.PID() }

// Write sends bytes to the shell stdin and records them (direction=stdin).
func (s *Session) Write(data []byte) error {
	if len(data) > 0 {
		if err := s.mgr.logMgr.Append(s.id, "stdin", data); err != nil {
			slog.Warn("terminal: log stdin", "id", s.id, "err", err)
		}
	}
	_, err := s.pty.Write(data)
	return err
}

// Resize changes the terminal window size.
func (s *Session) Resize(cols, rows int) error {
	return s.pty.Resize(cols, rows)
}

// SubscribeOutput registers a handler for persisted output chunks. The returned
// function unsubscribes.
func (s *Session) SubscribeOutput(h OutputHandler) (cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextSubID
	s.nextSubID++
	s.outputSubs[id] = h
	return func() {
		s.mu.Lock()
		delete(s.outputSubs, id)
		s.mu.Unlock()
	}
}

// SubscribeStatus registers a handler fired when the process exits. If the
// session already exited, the handler fires immediately.
func (s *Session) SubscribeStatus(h StatusHandler) (cancel func()) {
	s.mu.Lock()
	if s.exited {
		status, ec := s.status, s.exitCode
		s.mu.Unlock()
		go h(status, ec)
		return func() {}
	}
	id := s.nextSubID
	s.nextSubID++
	s.statusSubs[id] = h
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.statusSubs, id)
		s.mu.Unlock()
	}
}

// start launches the read loop goroutine.
func (s *Session) start() {
	s.wg.Add(1)
	go s.readLoop()
}

// readLoop pumps PTY output into the log store until the process exits.
func (s *Session) readLoop() {
	defer s.wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if aErr := s.mgr.logMgr.Append(s.id, "stdout", data); aErr != nil {
				slog.Warn("terminal: log stdout", "id", s.id, "err", aErr)
			}
		}
		if err != nil {
			s.finalize()
			return
		}
	}
}

// requestStop marks the session as user-stopped and closes the PTY, which
// unblocks the read loop and triggers finalize.
func (s *Session) requestStop() {
	s.mu.Lock()
	s.forceStop = true
	s.mu.Unlock()
	if err := s.pty.Close(); err != nil {
		slog.Debug("terminal: pty close on stop", "id", s.id, "err", err)
	}
}

// finalize records the exit status, flushes logs, notifies subscribers and
// detaches from the manager. Runs exactly once.
func (s *Session) finalize() {
	s.finalizeOnce.Do(func() {
		waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		code, waitErr := s.pty.Wait(waitCtx)
		cancel()

		s.mu.Lock()
		force := s.forceStop
		status := "exited"
		var ec *int
		if waitErr == nil {
			ec = &code
		} else if !force {
			status = "crashed"
		}
		s.status = status
		s.exitCode = ec
		s.exited = true
		statusSubs := make([]StatusHandler, 0, len(s.statusSubs))
		for _, h := range s.statusSubs {
			statusSubs = append(statusSubs, h)
		}
		s.statusSubs = nil
		s.mu.Unlock()

		updCtx, ucancel := context.WithTimeout(context.Background(), 3*time.Second)
		if uErr := s.mgr.termRepo.UpdateStatus(updCtx, s.id, status, ec); uErr != nil {
			slog.Warn("terminal: update exit status", "id", s.id, "err", uErr)
		}
		ucancel()

		for _, h := range statusSubs {
			h(status, ec)
		}

		s.mgr.logMgr.Close(s.id) // flush + stop this terminal's aggregators
		s.mgr.unregister(s.id)
		if cErr := s.pty.Close(); cErr != nil {
			slog.Debug("terminal: pty close", "id", s.id, "err", cErr)
		}
		close(s.done)
	})
}

// notifyOutput fans a flushed chunk out to output subscribers.
func (s *Session) notifyOutput(c logstore.FlushedChunk) {
	s.mu.RLock()
	subs := make([]OutputHandler, 0, len(s.outputSubs))
	for _, h := range s.outputSubs {
		subs = append(subs, h)
	}
	s.mu.RUnlock()
	for _, h := range subs {
		h(c)
	}
}
