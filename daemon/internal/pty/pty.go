// Package pty provides a cross-platform pseudo-terminal abstraction.
//
// Unix uses github.com/creack/pty; Windows uses the ConPTY API via
// github.com/UserExistsError/conpty. Callers use the uniform PTY interface
// regardless of platform.
package pty

import (
	"context"
	"os"
	"strings"
)

// PTY is a running pseudo-terminal attached to a child shell process.
type PTY interface {
	// Read reads merged stdout/stderr output as raw bytes (including ANSI
	// escape sequences). It blocks until data is available or the process
	// exits, in which case it returns an error (typically io.EOF).
	Read(p []byte) (int, error)
	// Write writes raw bytes to the shell's stdin.
	Write(p []byte) (int, error)
	// Resize changes the terminal window dimensions (cols x rows).
	Resize(cols, rows int) error
	// Wait blocks until the child process exits and returns its exit code.
	// On Windows the provided context can cancel the wait.
	Wait(ctx context.Context) (int, error)
	// Close releases pty resources and ensures the child is terminated.
	Close() error
	// PID returns the child process id (0 if unavailable).
	PID() int
}

// SpawnOpts configures a new pty.
type SpawnOpts struct {
	// Shell selects the shell. Accepted short names: cmd, powershell, pwsh,
	// bash, zsh, sh; or an absolute path / command line. Empty = platform
	// default (cmd.exe on Windows, $SHELL or /bin/sh on Unix).
	Shell string
	// Cwd is the initial working directory (empty = inherit).
	Cwd string
	// Cols and Rows are the initial terminal size (default 80x24).
	Cols int
	Rows int
	// Env are extra environment variables merged onto the daemon's environ.
	Env map[string]string
}

// Spawn starts a new pty running the configured shell.
func Spawn(opts SpawnOpts) (PTY, error) {
	if opts.Cols <= 0 {
		opts.Cols = 80
	}
	if opts.Rows <= 0 {
		opts.Rows = 24
	}
	return spawn(opts)
}

// mergedEnvSlice returns the current environment with extra entries applied
// (extra overrides). Returns nil when there are no extras so the platform can
// inherit the full parent environment untouched.
func mergedEnvSlice(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	envMap := make(map[string]string)
	for _, kv := range os.Environ() {
		if eq := strings.IndexByte(kv, '='); eq >= 0 {
			envMap[kv[:eq]] = kv[eq+1:]
		}
	}
	for k, v := range extra {
		envMap[k] = v
	}
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	return out
}
