package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// deadPID is a pid that is guaranteed not to exist on any supported platform.
const deadPID = 2000000000

func tmpLockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), LockFile)
}

func TestAcquireWritesLock(t *testing.T) {
	path := tmpLockPath(t)
	lk, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if lk.Token() == "" {
		t.Fatal("empty token")
	}
	if lk.Data().PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", lk.Data().PID, os.Getpid())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not written: %v", err)
	}
}

func TestAcquireStaleOverwrites(t *testing.T) {
	path := tmpLockPath(t)
	// a stale lock from a crashed daemon (dead pid)
	if err := atomicWrite(path, LockData{PID: deadPID, Port: 1234, Token: "old"}); err != nil {
		t.Fatal(err)
	}
	lk, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire stale: %v", err)
	}
	if lk.Data().PID != os.Getpid() {
		t.Fatal("stale lock was not overwritten")
	}
	if lk.Token() == "old" {
		t.Fatal("token not regenerated")
	}
}

func TestAcquireAliveFails(t *testing.T) {
	path := tmpLockPath(t)
	// a lock owned by this very test process (alive)
	if err := atomicWrite(path, LockData{PID: os.Getpid(), Port: 1234, Token: "mine"}); err != nil {
		t.Fatal(err)
	}
	_, err := Acquire(path)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestSetPort(t *testing.T) {
	path := tmpLockPath(t)
	lk, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	tok := lk.Token()
	if err := lk.SetPort(8080); err != nil {
		t.Fatal(err)
	}
	d, err := ReadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Port != 8080 {
		t.Fatalf("port = %d, want 8080", d.Port)
	}
	if d.Token != tok {
		t.Fatal("token changed on SetPort")
	}
}

func TestReleaseRemovesLock(t *testing.T) {
	path := tmpLockPath(t)
	lk, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lk.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("lock file still present after release")
	}
}

func TestReleaseKeepsOthersLock(t *testing.T) {
	path := tmpLockPath(t)
	lk, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	// simulate another instance taking over the lock
	if err := atomicWrite(path, LockData{PID: deadPID, Port: 9, Token: "other"}); err != nil {
		t.Fatal(err)
	}
	if err := lk.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("release wrongly removed another instance's lock")
	}
}

func TestProcessAliveSelf(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("current process should be reported alive")
	}
	if processAlive(deadPID) {
		t.Fatal("dead pid reported alive")
	}
}
