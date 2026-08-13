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

	"github.com/shellsync/daemon/internal/auth"
	"github.com/shellsync/daemon/internal/config"
	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/lifecycle"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/repository"
	"github.com/shellsync/daemon/internal/service"
	"github.com/shellsync/daemon/internal/terminal"
	transporthttp "github.com/shellsync/daemon/internal/transport/http"
	"github.com/shellsync/daemon/internal/transport/ws"
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
	startedAt := time.Now()

	// --- M1-2: configuration ---
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg)
	slog.Info("config loaded",
		"dataDir", cfg.DataDir,
		"httpPort", cfg.HTTPPort,
		"logLevel", cfg.LogLevel,
		"logRetention", cfg.LogRetention,
	)

	// --- M1-3: single instance lock ---
	lock, err := lifecycle.Acquire(lifecycle.LockPath(cfg.DataDir))
	if err != nil {
		if errors.Is(err, lifecycle.ErrAlreadyRunning) {
			slog.Info("another daemon instance is already running, exiting", "err", err)
			fmt.Println("ShellSync daemon is already running.")
			return nil
		}
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()
	slog.Info("lock acquired", "path", lock.Path(), "pid", os.Getpid())

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
	taskRepo := repository.NewTaskRepo(db)
	terminalRepo := repository.NewTerminalRepo(db)
	todoRepo := repository.NewTodoRepo(db)
	logRepo := repository.NewLogRepo(db)
	deviceRepo := repository.NewDeviceRepo(db)
	settingsRepo := repository.NewSettingsRepo(db)

	logMgr := logstore.NewManager(logRepo, logstore.Config{
		FlushWindow:   16 * time.Millisecond,
		MaxChunkBytes: 16 * 1024,
		LogsPath:      cfg.LogsPath(),
	})
	defer logMgr.CloseAll()

	termMgr := terminal.NewManager(terminalRepo, logMgr)
	defer termMgr.CloseAll()

	if recovered, err := termMgr.RecoverOnStartup(ctx); err != nil {
		return fmt.Errorf("recover: %w", err)
	} else if recovered > 0 {
		slog.Info("recovered stale terminals", "count", recovered)
	}

	// --- M2: services / auth / events / servers ---
	bus := eventbus.New()
	svc := service.New(service.Deps{
		UserID:       repository.DefaultUserID,
		TaskRepo:     taskRepo,
		TerminalRepo: terminalRepo,
		TodoRepo:     todoRepo,
		LogRepo:      logRepo,
		DeviceRepo:   deviceRepo,
		SettingsRepo: settingsRepo,
		TermMgr:      termMgr,
		LogMgr:       logMgr,
		Bus:          bus,
	})
	verifier := auth.NewVerifier(lock.Token(), deviceRepo)
	wsHub := ws.NewHub(verifier, svc.Terminals, logMgr, bus)

	httpServer := transporthttp.New(transporthttp.Deps{
		Version:   Version,
		StartedAt: startedAt,
		Svc:       svc,
		Auth:      verifier,
		WS:        wsHub,
		Shutdown:  stop,
	})

	port, shutdownHTTP, err := transporthttp.Serve(httpServer.Handler(), cfg.HTTPPort)
	if err != nil {
		return fmt.Errorf("serve http: %w", err)
	}
	if err := lock.SetPort(port); err != nil {
		slog.Warn("write port to lock", "err", err)
	}
	svc.Pair.SetPort(port)
	slog.Info("http listening", "port", port, "token", lock.Token())

	fmt.Printf("ShellSync daemon %s ready (pid=%d, port=%d, data=%s)\n", Version, os.Getpid(), port, cfg.DataDir)

	<-ctx.Done()
	slog.Info("shutdown signal received, exiting gracefully")

	shutCtx, cancelShut := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShut()
	if err := shutdownHTTP(shutCtx); err != nil {
		slog.Warn("http shutdown", "err", err)
	}
	return nil
}

// setupLogger configures the global slog logger from the config.
func setupLogger(cfg config.Config) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	})))
}
