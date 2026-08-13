// Package main is the entry point of the ShellSync daemon.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shellsync/daemon/internal/config"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/repository"
	"github.com/shellsync/daemon/internal/terminal"
)

// Version is the daemon version. It is overridden at build time via
// -ldflags "-X main.Version=...". The default "dev" is used for local builds.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("daemon exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("daemon stopped")
}

func run() error {
	// --- M1-2: configuration ---
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg)
	slog.Info("config loaded",
		"dataDir", cfg.DataDir,
		"httpPort", cfg.HTTPPort,
		"wsPort", cfg.WSPort,
		"logLevel", cfg.LogLevel,
		"logRetention", cfg.LogRetention,
	)

	// --- M1-4: database ---
	db, err := repository.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			slog.Error("close db", "err", cErr)
		}
	}()
	slog.Info("database opened", "path", cfg.DBPath())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := repository.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := repository.SeedDefaults(ctx, db); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	slog.Info("database ready")

	// --- M1-5/7/8: terminal subsystem ---
	termRepo := repository.NewTerminalRepo(db)
	logRepo := repository.NewLogRepo(db)
	logMgr := logstore.NewManager(logRepo, logstore.Config{
		FlushWindow:   16 * time.Millisecond,
		MaxChunkBytes: 16 * 1024,
		LogsPath:      cfg.LogsPath(),
	})
	defer logMgr.CloseAll()

	termMgr := terminal.NewManager(termRepo, logMgr)
	defer termMgr.CloseAll()

	recovered, err := termMgr.RecoverOnStartup(ctx)
	if err != nil {
		return fmt.Errorf("recover: %w", err)
	}
	if recovered > 0 {
		slog.Info("recovered stale terminals", "count", recovered)
	}

	// TODO(M1-3): acquire lock file (single instance) + structured shutdown wiring
	// TODO(M2): mount HTTP + WebSocket servers

	slog.Info("daemon started", "version", Version, "pid", os.Getpid())
	fmt.Printf("ShellSync daemon %s ready (pid=%d, data=%s)\n", Version, os.Getpid(), cfg.DataDir)

	<-ctx.Done()
	slog.Info("shutdown signal received, exiting gracefully")
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// setupLogger configures the global slog logger from the config.
func setupLogger(cfg config.Config) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	})))
}
