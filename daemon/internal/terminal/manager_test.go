package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/repository"
)

func setupManager(t *testing.T) (*Manager, *repository.TerminalRepo, *repository.LogRepo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := repository.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := repository.SeedDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	termRepo := repository.NewTerminalRepo(db)
	logRepo := repository.NewLogRepo(db)
	logMgr := logstore.NewManager(logRepo, logstore.Config{
		FlushWindow: 20 * time.Millisecond,
		LogsPath:    filepath.Join(t.TempDir(), "logs"),
	})
	t.Cleanup(logMgr.CloseAll)

	mgr := NewManager(termRepo, logMgr)
	t.Cleanup(mgr.CloseAll)
	return mgr, termRepo, logRepo
}

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal(msg)
}

func shellForOS() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "bash"
}

const m1_8_marker = "SHELLSYNC_M1_8_MARKER"

// Full lifecycle: create (real shell) -> write echo -> receive via subscriber
// -> logs persisted -> stop -> DB status "exited" -> session removed.
func TestSessionCreateEchoStop(t *testing.T) {
	mgr, termRepo, logRepo := setupManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateOpts{UserID: repository.DefaultUserID, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	meta, err := termRepo.Get(ctx, sess.ID())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != "running" {
		t.Fatalf("status = %q, want running", meta.Status)
	}

	var mu sync.Mutex
	var collected []byte
	unsub := sess.SubscribeOutput(func(c logstore.FlushedChunk) {
		raw, _ := base64.StdEncoding.DecodeString(c.ContentB64)
		mu.Lock()
		collected = append(collected, raw...)
		mu.Unlock()
	})
	defer unsub()

	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	if err := sess.Write([]byte("echo " + m1_8_marker + nl)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The echoed input (and/or the command's output) must reach subscribers.
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return bytes.Contains(collected, []byte(m1_8_marker))
	}, 4*time.Second, "marker not seen in output")

	waitFor(t, func() bool {
		max, _ := logRepo.MaxSeq(ctx, sess.ID())
		return max > 0
	}, 2*time.Second, "no logs persisted")

	if err := mgr.Stop(sess.ID()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	meta, _ = termRepo.Get(ctx, sess.ID())
	if meta.Status != "exited" && meta.Status != "crashed" {
		t.Fatalf("status after stop = %q, want exited or crashed", meta.Status)
	}
	if _, ok := mgr.Get(sess.ID()); ok {
		t.Fatal("session still live after stop")
	}
}

// Stop then Restart re-spawns the same terminal id and flips it back to running.
func TestRestart(t *testing.T) {
	mgr, termRepo, _ := setupManager(t)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, CreateOpts{UserID: repository.DefaultUserID, ShellType: shellForOS()})
	if err != nil {
		t.Fatal(err)
	}
	id := sess.ID()
	if err := mgr.Stop(id); err != nil {
		t.Fatal(err)
	}

	sess2, err := mgr.Restart(ctx, id)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if sess2.ID() != id {
		t.Fatalf("id changed: %q -> %q", id, sess2.ID())
	}
	meta, _ := termRepo.Get(ctx, id)
	if meta.Status != "running" {
		t.Fatalf("status = %q, want running", meta.Status)
	}
	mgr.Stop(id)
}

// RecoverOnStartup flips stale "running" rows to "crashed" and leaves others.
func TestRecoverOnStartup(t *testing.T) {
	mgr, termRepo, _ := setupManager(t)
	ctx := context.Background()

	running, _ := termRepo.Create(ctx, repository.TerminalCreate{UserID: repository.DefaultUserID, ShellType: "bash"})
	exited, _ := termRepo.Create(ctx, repository.TerminalCreate{UserID: repository.DefaultUserID, ShellType: "bash"})
	if err := termRepo.UpdateStatus(ctx, exited.ID, "exited", nil); err != nil {
		t.Fatal(err)
	}

	n, err := mgr.RecoverOnStartup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}
	got, _ := termRepo.Get(ctx, running.ID)
	if got.Status != "crashed" {
		t.Fatalf("running -> %q, want crashed", got.Status)
	}
	got2, _ := termRepo.Get(ctx, exited.ID)
	if got2.Status != "exited" {
		t.Fatalf("exited touched -> %q", got2.Status)
	}
}
