//go:build !windows

package pty

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
)

type unixPTY struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func spawn(opts SpawnOpts) (PTY, error) {
	exe, args := resolveShell(opts.Shell)
	cmd := exec.Command(exe, args...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	if env := mergedEnvSlice(opts.Env); env != nil {
		cmd.Env = env
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(opts.Cols),
		Rows: uint16(opts.Rows),
	})
	if err != nil {
		return nil, fmt.Errorf("pty: start %q: %w", exe, err)
	}
	return &unixPTY{ptmx: ptmx, cmd: cmd}, nil
}

func (p *unixPTY) Read(b []byte) (int, error)  { return p.ptmx.Read(b) }
func (p *unixPTY) Write(b []byte) (int, error) { return p.ptmx.Write(b) }

func (p *unixPTY) Resize(cols, rows int) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *unixPTY) Wait(_ context.Context) (int, error) {
	// exec.Cmd.Wait does not accept a context; ctx is ignored.
	if p.cmd.Process == nil {
		return -1, nil
	}
	err := p.cmd.Wait()
	if p.cmd.ProcessState != nil {
		return p.cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func (p *unixPTY) Close() error {
	// Close the master (sends SIGHUP to the child) and SIGKILL it as a safety
	// net. We deliberately do NOT reap here: Wait() is the single reaper and is
	// invoked by the terminal manager's finalize step. Reaping in both places
	// would make the second Wait fail and misclassify the exit as "crashed".
	err := p.ptmx.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return err
}

func (p *unixPTY) PID() int {
	if p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// resolveShell maps a shell name to an executable path on Unix.
func resolveShell(shell string) (exe string, args []string) {
	if shell == "" {
		if s := os.Getenv("SHELL"); s != "" {
			return s, nil
		}
		return "/bin/sh", nil
	}
	if strings.ContainsRune(shell, '/') {
		return shell, nil // treat as a path
	}
	if path, err := exec.LookPath(shell); err == nil {
		return path, nil
	}
	switch shell {
	case "bash":
		return "/bin/bash", nil
	case "zsh":
		return "/bin/zsh", nil
	default:
		return shell, nil
	}
}
