// Package lifecycle manages the daemon process lifecycle: single-instance
// enforcement via a lock file, and graceful shutdown wiring.
package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockFile is the name of the daemon lock file inside the data directory.
const LockFile = "daemon.lock"

// ErrAlreadyRunning indicates another live daemon instance owns the lock.
var ErrAlreadyRunning = errors.New("shellsync daemon is already running")

// LockData is the JSON content of the lock file.
type LockData struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`      // 0 until the HTTP server binds (M2)
	Token     string `json:"token"`     // local auth token for the Desktop client
	StartedAt int64  `json:"startedAt"` // unix millis
}

// Lock represents ownership of the daemon lock file.
type Lock struct {
	path string
	data LockData
}

// Path returns the lock file path.
func (l *Lock) Path() string { return l.path }

// Token returns the local authentication token.
func (l *Lock) Token() string { return l.data.Token }

// Port returns the bound HTTP port (0 until SetPort is called).
func (l *Lock) Port() int { return l.data.Port }

// Data returns a copy of the lock data.
func (l *Lock) Data() LockData { return l.data }

// Acquire checks for a running instance and, if none, writes the lock file.
// path is typically LockPath(dataDir).
//
// If a lock exists and its process is still alive, Acquire returns an error
// wrapping ErrAlreadyRunning. A stale lock (from a crashed daemon) is removed
// and replaced.
func Acquire(path string) (*Lock, error) {
	if existing, err := readLock(path); err == nil {
		if processAlive(existing.PID) {
			return nil, fmt.Errorf("%w (pid=%d, port=%d)", ErrAlreadyRunning, existing.PID, existing.Port)
		}
		// stale lock from a previous crash — clear it
		_ = os.Remove(path)
	}

	data := LockData{
		PID:       os.Getpid(),
		Token:     randToken(),
		StartedAt: time.Now().UnixMilli(),
	}
	if err := atomicWrite(path, data); err != nil {
		return nil, fmt.Errorf("write lock: %w", err)
	}
	return &Lock{path: path, data: data}, nil
}

// SetPort updates the port in the lock file (call once the HTTP server binds).
func (l *Lock) SetPort(port int) error {
	l.data.Port = port
	return atomicWrite(l.path, l.data)
}

// Release removes the lock file, but only if it still belongs to this process.
// If another instance has taken over, the lock is left untouched.
func (l *Lock) Release() error {
	current, err := readLock(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if current.PID != l.data.PID {
		return nil // someone else owns it now
	}
	return os.Remove(l.path)
}

// ReadLock reads and parses a lock file. Useful for clients (e.g. the Desktop
// launcher) to discover the running daemon's port and token.
func ReadLock(path string) (LockData, error) {
	return readLock(path)
}

// LockPath joins a data directory to the lock file name.
func LockPath(dataDir string) string {
	return filepath.Join(dataDir, LockFile)
}

func readLock(path string) (LockData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return LockData{}, err
	}
	var d LockData
	if err := json.Unmarshal(b, &d); err != nil {
		return LockData{}, fmt.Errorf("parse lock %s: %w", path, err)
	}
	return d, nil
}

// atomicWrite writes the lock via a temp file + rename for crash safety.
func atomicWrite(path string, data LockData) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// extremely unlikely; fall back to a time-based value
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(b)
}
