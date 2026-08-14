package service

import (
	"context"

	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/repository"
	"github.com/shellsync/daemon/internal/terminal"
)

// TerminalService bridges the live terminal.Manager with persistence and
// emits change events.
type TerminalService struct {
	mgr    *terminal.Manager
	repo   *repository.TerminalRepo
	logMgr *logstore.Manager
	bus    *eventbus.Bus
	userID string
}

// Create spawns a terminal.
func (s *TerminalService) Create(ctx context.Context, in terminal.CreateOpts) (*terminal.Session, error) {
	in.UserID = s.userID
	sess, err := s.mgr.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	if t, err := s.repo.Get(ctx, sess.ID()); err == nil {
		s.bus.Publish(terminalEvent("created", t))
	}
	return sess, nil
}

// Get returns terminal metadata.
func (s *TerminalService) Get(ctx context.Context, id string) (repository.Terminal, error) {
	return s.repo.Get(ctx, id)
}

// List returns the user's terminals.
func (s *TerminalService) List(ctx context.Context, f repository.TerminalFilter) ([]repository.Terminal, error) {
	return s.mgr.List(ctx, s.userID, f)
}

// Update patches a terminal (name / task binding).
func (s *TerminalService) Update(ctx context.Context, id string, p repository.TerminalPatch) (repository.Terminal, error) {
	t, err := s.repo.Update(ctx, id, p)
	if err != nil {
		return repository.Terminal{}, err
	}
	s.bus.Publish(terminalEvent("updated", t))
	return t, nil
}

// Stop closes the live session (marks the terminal exited/crashed).
func (s *TerminalService) Stop(ctx context.Context, id string) error {
	if err := s.mgr.Stop(id); err != nil {
		return err
	}
	if t, err := s.repo.Get(ctx, id); err == nil {
		s.bus.Publish(terminalEvent("updated", t))
	}
	return nil
}

// Delete stops the live session (if any) and removes the terminal record;
// its logs cascade-delete via FK.
func (s *TerminalService) Delete(ctx context.Context, id string) error {
	// mgr.Stop is a no-op when the session is not running.
	if err := s.mgr.Stop(id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.bus.Publish(deleteEvent("terminal", id))
	return nil
}

// Restart re-spawns an exited/crashed terminal. If the session is still live
// it is stopped first (a user restart on a running terminal is expected to
// respawn it, not fail).
func (s *TerminalService) Restart(ctx context.Context, id string) (*terminal.Session, error) {
	if _, ok := s.mgr.Get(id); ok {
		if err := s.mgr.Stop(id); err != nil {
			return nil, err
		}
	}
	sess, err := s.mgr.Restart(ctx, id)
	if err != nil {
		return nil, err
	}
	if t, err := s.repo.Get(ctx, id); err == nil {
		s.bus.Publish(terminalEvent("updated", t))
	}
	return sess, nil
}

// Session returns the live session for a terminal (ok=false if not running).
func (s *TerminalService) Session(id string) (*terminal.Session, bool) {
	return s.mgr.Get(id)
}

// LogMgr exposes the log store (used by the WS layer for history).
func (s *TerminalService) LogMgr() *logstore.Manager { return s.logMgr }

// Logs returns a page of history chunks (seq >= fromSeq).
func (s *TerminalService) Logs(ctx context.Context, terminalID string, fromSeq int64, limit int) ([]repository.TerminalLog, bool, error) {
	return s.logMgr.ReadRange(ctx, terminalID, fromSeq, limit)
}

// LogTail returns the newest chunks in ascending order.
func (s *TerminalService) LogTail(ctx context.Context, terminalID string, limit int) ([]repository.TerminalLog, error) {
	return s.logMgr.Tail(ctx, terminalID, limit)
}
