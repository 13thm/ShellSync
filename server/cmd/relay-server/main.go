// Package main is the relay-server entry point: config → logger → router →
// listen, with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shellsync/relay-server/internal/config"
	relayhttp "github.com/shellsync/relay-server/internal/transport/http"
	"github.com/shellsync/relay-server/relay"
)

// Version is overridden at build time via
// -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("relay-server exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("relay-server stopped")
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg)
	slog.Info("config loaded", "listen", cfg.Listen, "tls", cfg.TLS, "version", Version)
	if cfg.TLS {
		return errors.New("in-process TLS is not supported in R1 — terminate TLS at Caddy (see deploy/relay)")
	}

	hub := relay.NewHub(slog.Default(), nil)
	srv := relayhttp.New(relayhttp.Deps{Version: Version, Hub: hub, Log: slog.Default()})

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	lnErr := make(chan error, 1)
	go func() { lnErr <- httpServer.ListenAndServe() }()
	slog.Info("relay listening", "addr", cfg.Listen)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-lnErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections")
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutCtx); err != nil {
		slog.Warn("http shutdown", "err", err)
	}
	hub.Close() // closes remaining sessions, notifying peers
	return nil
}

func setupLogger(cfg config.Config) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	})))
}
