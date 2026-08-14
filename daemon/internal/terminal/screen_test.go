package terminal

import (
	"strings"
	"testing"
)

func TestScreenRendersTUI(t *testing.T) {
	s := NewScreen(40, 5)
	// simulate a TUI frame: clear screen, cursor home, draw a box line + text at col 15
	s.Write([]byte("\x1b[2J\x1b[H"))              // clear + home
	s.Write([]byte("Welcome to claude code\r\n")) // top line
	s.Write([]byte("\x1b[3;15HTUI renders here")) // absolute cursor positioning
	lines, cols, rows := s.Snapshot()
	if cols != 40 || rows != 5 {
		t.Fatalf("size = %dx%d, want 40x5", cols, rows)
	}
	if len(lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(lines))
	}
	if !strings.HasPrefix(lines[0], "Welcome to claude code") {
		t.Fatalf("line0 = %q", lines[0])
	}
	if !strings.HasPrefix(strings.TrimRight(lines[2], " "), "              TUI renders here") {
		t.Fatalf("line2 (abs positioned) = %q", lines[2])
	}
}

func TestScreenDirty(t *testing.T) {
	s := NewScreen(80, 24)
	_, _, _ = s.Snapshot() // clears dirty
	if s.IsDirty() {
		t.Fatal("expected clean after snapshot")
	}
	s.Write([]byte("hi"))
	if !s.IsDirty() {
		t.Fatal("expected dirty after write")
	}
}
