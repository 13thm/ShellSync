package terminal

import (
	"strings"
	"sync"

	"github.com/hinshun/vt10x"
)

// Screen is a server-side terminal emulator mirroring the PTY output.
// Mobile clients request the *rendered* screen (plain lines) instead of
// replaying raw escape sequences, which guarantees TUI apps (claude, vim…)
// display correctly on any device regardless of its viewport size.
type Screen struct {
	mu      sync.Mutex
	term    vt10x.Terminal
	pending []byte // leftover partial UTF-8 sequence between writes
	dirty   bool
}

// NewScreen creates an emulator with the given initial size (defaults 80x24).
func NewScreen(cols, rows int) *Screen {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return &Screen{term: vt10x.New(vt10x.WithSize(cols, rows)), dirty: true}
}

// Write feeds PTY output into the emulator. Partial UTF-8 sequences at chunk
// boundaries are buffered until completed.
func (s *Screen) Write(p []byte) {
	s.mu.Lock()
	s.pending = append(s.pending, p...)
	n, err := s.term.Write(s.pending)
	if err == nil && n >= 0 && n <= len(s.pending) {
		if n < len(s.pending) {
			rest := make([]byte, len(s.pending)-n)
			copy(rest, s.pending[n:])
			s.pending = rest
		} else {
			s.pending = s.pending[:0]
		}
	}
	s.dirty = true
	s.mu.Unlock()
}

// Resize resizes the emulator grid.
func (s *Screen) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s.mu.Lock()
	s.term.Resize(cols, rows)
	s.dirty = true
	s.mu.Unlock()
}

// IsDirty reports whether the screen changed since the last Snapshot.
func (s *Screen) IsDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dirty
}

// IsAltScreen reports whether the terminal is currently in alternate-screen
// mode (TUI apps like claude/vim set this). Clients use it to pick between a
// "live TUI" snapshot view and a scrollback log view.
func (s *Screen) IsAltScreen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.term.Mode()&vt10x.ModeAltScreen != 0
}

// Snapshot returns the visible screen lines (top to bottom) and grid size.
func (s *Screen) Snapshot() (lines []string, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = false
	content := s.term.String()
	lines = strings.Split(strings.TrimRight(content, "\n"), "\n")
	cols, rows = s.term.Size()
	return lines, cols, rows
}
