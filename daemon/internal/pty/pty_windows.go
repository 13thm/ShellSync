//go:build windows

package pty

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/UserExistsError/conpty"
)

type windowsPTY struct {
	cpty      *conpty.ConPty
	closeOnce sync.Once
	closeErr  error
}

func spawn(opts SpawnOpts) (PTY, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, fmt.Errorf("pty: ConPTY not available (requires Windows 10 1809+)")
	}
	cmdline := resolveCommandLine(opts.Shell)

	conOpts := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(opts.Cols, opts.Rows),
	}
	if opts.Cwd != "" {
		conOpts = append(conOpts, conpty.ConPtyWorkDir(opts.Cwd))
	}
	if env := mergedEnvSlice(opts.Env); len(env) > 0 {
		conOpts = append(conOpts, conpty.ConPtyEnv(env))
	}

	cpty, err := conpty.Start(cmdline, conOpts...)
	if err != nil {
		return nil, fmt.Errorf("pty: start %q: %w", cmdline, err)
	}
	return &windowsPTY{cpty: cpty}, nil
}

func (p *windowsPTY) Read(b []byte) (int, error)  { return p.cpty.Read(b) }
func (p *windowsPTY) Write(b []byte) (int, error) { return p.cpty.Write(b) }
func (p *windowsPTY) Resize(cols, rows int) error { return p.cpty.Resize(cols, rows) }
func (p *windowsPTY) PID() int                    { return p.cpty.Pid() }

func (p *windowsPTY) Wait(ctx context.Context) (int, error) {
	var code int
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pty wait panic: %v", r)
			slog.Debug("pty: recovered wait panic", "err", r)
		}
	}()
	c, e := p.cpty.Wait(ctx)
	code, err = int(c), e
	return code, err
}

func (p *windowsPTY) Close() error {
	p.closeOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				p.closeErr = fmt.Errorf("pty close panic: %v", r)
				slog.Debug("pty: recovered close panic", "err", r)
			}
		}()
		p.closeErr = p.cpty.Close()
	})
	return p.closeErr
}

// resolveCommandLine maps a shell name to a Windows command line.
func resolveCommandLine(shell string) string {
	switch strings.ToLower(shell) {
	case "", "cmd", "cmd.exe":
		return "cmd.exe"
	case "powershell", "powershell.exe":
		return "powershell.exe"
	case "pwsh", "pwsh.exe":
		return "pwsh.exe"
	case "bash", "bash.exe":
		return "bash.exe"
	default:
		return shell // treat as an absolute path / command line
	}
}
