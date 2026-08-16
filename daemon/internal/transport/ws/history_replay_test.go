package ws

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/shellsync/daemon/internal/terminal"
)

// subscribeAndCollect subscribes to a terminal on a fresh connection
// (simulating a client that left the view and came back) and returns the
// concatenated decoded history plus every chunk seq received.
func subscribeAndCollect(t *testing.T, hub *Hub, tok, termID string) (history string, seqs []int64, gotSubscribed bool) {
	t.Helper()
	c := dialWS(t, hub, tok)
	writeMsg(t, c, map[string]any{
		"type": "terminal.subscribe", "id": "s1",
		"payload": map[string]any{"terminalId": termID},
	})

	var hist strings.Builder
	var out []int64
	for i := 0; i < 5000; i++ {
		m := readMsg(t, c)
		switch m["type"] {
		case "terminal.history":
			p := m["payload"].(map[string]any)
			for _, cc := range p["chunks"].([]any) {
				ch := cc.(map[string]any)
				out = append(out, int64(ch["seq"].(float64)))
				raw, err := base64.StdEncoding.DecodeString(ch["contentB64"].(string))
				if err != nil {
					t.Fatalf("decode history chunk: %v", err)
				}
				hist.Write(raw)
			}
		case "terminal.subscribed":
			return hist.String(), out, true
		}
	}
	return hist.String(), out, false
}

// waitEcho subscribes a live client, sends a command and blocks until the
// command's output is flushed back (which also means it is persisted).
func waitEcho(t *testing.T, hub *Hub, tok, termID, cmd, mark string) {
	t.Helper()
	c := dialWS(t, hub, tok)
	writeMsg(t, c, map[string]any{"type": "terminal.subscribe", "id": "a", "payload": map[string]any{"terminalId": termID}})
	writeMsg(t, c, map[string]any{"type": "terminal.input", "payload": map[string]any{
		"terminalId": termID, "dataB64": base64.StdEncoding.EncodeToString([]byte(cmd + "\r")),
	}})
	for i := 0; i < 100; i++ {
		m := readMsg(t, c)
		if m["type"] == "terminal.output" {
			p := m["payload"].(map[string]any)
			raw, _ := base64.StdEncoding.DecodeString(p["contentB64"].(string))
			if strings.Contains(string(raw), mark) {
				// let the logstore persist any trailing partial chunk
				time.Sleep(100 * time.Millisecond)
				return
			}
		}
	}
	t.Fatalf("never saw %q in live output", mark)
}

// Repro: leaving the terminal view and coming back must replay the full
// history (command + echoed input + output) for a still-running session.
func TestHistoryReplayOnReenterRunning(t *testing.T) {
	hub, svc, tok := setupHub(t)
	ctx := context.Background()

	sess, err := svc.Terminals.Create(ctx, terminal.CreateOpts{ShellType: "bash", Name: "t"})
	if err != nil {
		t.Fatal(err)
	}
	id := sess.ID()

	waitEcho(t, hub, tok, id, "echo hello-mark", "hello-mark")

	// "exit and re-enter": brand-new connection subscribes
	hist, _, ok := subscribeAndCollect(t, hub, tok, id)
	if !ok {
		t.Fatal("no terminal.subscribed reply")
	}
	if !strings.Contains(hist, "hello-mark") {
		t.Fatalf("replayed history misses the command output; got %d bytes", len(hist))
	}
	if !strings.Contains(hist, "echo hello-mark") {
		t.Fatalf("replayed history misses the echoed command; got %d bytes", len(hist))
	}
}

// Repro: after the shell exits, re-entering must still replay the history.
func TestHistoryReplayAfterSessionExit(t *testing.T) {
	hub, svc, tok := setupHub(t)
	ctx := context.Background()

	sess, err := svc.Terminals.Create(ctx, terminal.CreateOpts{ShellType: "bash", Name: "t"})
	if err != nil {
		t.Fatal(err)
	}
	id := sess.ID()

	waitEcho(t, hub, tok, id, "echo before-exit", "before-exit")

	// stop the session (user exits the shell)
	if err := svc.Terminals.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	hist, _, ok := subscribeAndCollect(t, hub, tok, id)
	if !ok {
		t.Fatal("no terminal.subscribed reply")
	}
	if !strings.Contains(hist, "before-exit") {
		t.Fatalf("history after exit is missing output; got %d bytes", len(hist))
	}
}

// Repro: a session with more than one page (500 chunks) of history must have
// every page replayed — this is the "middle content missing" bug. Verifies
// every persisted seq arrives exactly once (no gaps, no duplicates).
func TestHistoryReplayAllPages(t *testing.T) {
	hub, svc, tok := setupHub(t)
	ctx := context.Background()

	sess, err := svc.Terminals.Create(ctx, terminal.CreateOpts{ShellType: "bash", Name: "t"})
	if err != nil {
		t.Fatal(err)
	}
	id := sess.ID()

	// push >500 distinct chunks directly through the log manager
	lm := svc.Terminals.LogMgr()
	for i := 0; i < 620; i++ {
		if err := lm.Append(id, "stdout", []byte{byte('a' + i%26)}); err != nil {
			t.Fatal(err)
		}
		// sleep past the test flush window so each append becomes its own chunk
		time.Sleep(25 * time.Millisecond)
	}
	// flush the tail
	lm.Close(id)

	_, seqs, ok := subscribeAndCollect(t, hub, tok, id)
	if !ok {
		t.Fatal("no terminal.subscribed reply")
	}
	if len(seqs) < 620 {
		t.Fatalf("got %d chunks, want >= 620 (multi-page replay incomplete)", len(seqs))
	}
	// every seq must arrive exactly once
	seen := map[int64]int{}
	for _, s := range seqs {
		seen[s]++
		if seen[s] > 1 {
			t.Fatalf("seq %d received %d times", s, seen[s])
		}
	}
	for s := int64(1); s <= int64(len(seqs)); s++ {
		if seen[s] == 0 {
			t.Fatalf("seq %d missing (gap in replay)", s)
		}
	}
}

// Resize markers (direction="resize") must be included in history replay so
// clients can re-grid their emulator at the right point; stdin chunks must
// stay excluded.
func TestHistoryReplayIncludesResizeMarkers(t *testing.T) {
	hub, svc, tok := setupHub(t)
	ctx := context.Background()

	sess, err := svc.Terminals.Create(ctx, terminal.CreateOpts{ShellType: "bash", Name: "t"})
	if err != nil {
		t.Fatal(err)
	}
	// A client window change logs a resize marker; output follows at that size.
	if err := sess.Resize(80, 24); err != nil {
		t.Fatal(err)
	}
	waitEcho(t, hub, tok, sess.ID(), "echo marker-after-resize", "marker-after-resize")

	c := dialWS(t, hub, tok)
	writeMsg(t, c, map[string]any{
		"type": "terminal.subscribe", "id": "s1",
		"payload": map[string]any{"terminalId": sess.ID()},
	})
	var resizeSeen, stdinSeen bool
	for i := 0; i < 5000; i++ {
		m := readMsg(t, c)
		if m["type"] == "terminal.subscribed" {
			break
		}
		if m["type"] != "terminal.history" {
			continue
		}
		p := m["payload"].(map[string]any)
		for _, cc := range p["chunks"].([]any) {
			ch := cc.(map[string]any)
			switch ch["direction"] {
			case "resize":
				resizeSeen = true
			case "stdin":
				stdinSeen = true
			}
		}
	}
	if !resizeSeen {
		t.Fatal("resize marker missing from history replay")
	}
	if stdinSeen {
		t.Fatal("stdin chunks must not be replayed")
	}
}
