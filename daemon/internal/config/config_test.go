package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.DataDir == "" {
		t.Fatal("default DataDir is empty")
	}
	if c.LogRetention <= 0 {
		t.Fatal("default LogRetention should be positive")
	}
	if c.Version != ConfigVersion {
		t.Fatalf("version = %d, want %d", c.Version, ConfigVersion)
	}
	if c.SlogLevel() != slog.LevelInfo {
		t.Fatalf("default level = %v, want Info", c.SlogLevel())
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv(EnvDataDir, "/tmp/shellsync-env-override")
	c := Default()
	if c.DataDir != "/tmp/shellsync-env-override" {
		t.Fatalf("expected env override, got %q", c.DataDir)
	}
}

func TestLoadMissingCreatesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDataDir, dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != dir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	// config.json should have been written.
	if _, err := os.Stat(cfg.FilePath()); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	// logs dir should have been created.
	if _, err := os.Stat(cfg.LogsPath()); err != nil {
		t.Fatalf("logs dir not created: %v", err)
	}
}

func TestLoadReadsExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDataDir, dir)

	seed := Default()
	seed.LogRetention = 30
	seed.LogLevel = "debug"
	if err := seed.WriteFile(seed.FilePath()); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LogRetention != 30 {
		t.Fatalf("LogRetention = %d, want 30", got.LogRetention)
	}
	if got.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", got.LogLevel)
	}
	if got.SlogLevel() != slog.LevelDebug {
		t.Fatalf("level = %v, want Debug", got.SlogLevel())
	}
}

func TestEnsureDirs(t *testing.T) {
	c := Default()
	c.DataDir = filepath.Join(t.TempDir(), "data")
	if err := c.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	if _, err := os.Stat(c.LogsPath()); err != nil {
		t.Fatalf("logs dir missing: %v", err)
	}
}
