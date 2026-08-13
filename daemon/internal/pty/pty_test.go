package pty

import (
	"bytes"
	"runtime"
	"testing"
	"time"
)

const marker = "shellsync_pty_test_marker"

// TestSpawnEchoReadResize exercises the full pty lifecycle on the current
// platform: spawn the default shell, echo a marker, read it back, resize and
// close. It runs on every OS (the shell/line-ending are chosen accordingly).
func TestSpawnEchoReadResize(t *testing.T) {
	p, err := Spawn(SpawnOpts{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		if cErr := p.Close(); cErr != nil {
			t.Logf("Close: %v", cErr)
		}
	})

	// cmd.exe needs CRLF; POSIX shells want LF.
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	if _, err := p.Write([]byte("echo " + marker + nl)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := readFor(p, marker, 4*time.Second)
	if !bytes.Contains(got, []byte(marker)) {
		t.Fatalf("marker not found in output within timeout.\noutput: %q", got)
	}

	if err := p.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if pid := p.PID(); pid <= 0 {
		t.Fatalf("PID = %d, want > 0", pid)
	}
}

// readFor reads from p until needle appears in the accumulated output or the
// timeout elapses (or the read loop ends). Only the main goroutine touches the
// buffer, so it is race-free.
func readFor(p PTY, needle string, timeout time.Duration) []byte {
	type readResult struct {
		data []byte
		err  error
	}
	chunks := make(chan readResult, 16)
	go func() {
		tmp := make([]byte, 4096)
		for {
			n, err := p.Read(tmp)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, tmp[:n])
				chunks <- readResult{data: cp, err: err}
			}
			if err != nil {
				return
			}
		}
	}()

	var buf bytes.Buffer
	deadline := time.Now().Add(timeout)
	for {
		if bytes.Contains(buf.Bytes(), []byte(needle)) {
			return buf.Bytes()
		}
		if d := time.Until(deadline); d <= 0 {
			return buf.Bytes()
		}
		select {
		case r := <-chunks:
			buf.Write(r.data)
			if r.err != nil {
				return buf.Bytes()
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
}
