// Package config tests: defaults, file overlay, env override.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	t.Setenv(EnvListen, "")
	t.Setenv(EnvName, "")
	cfg, err := load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != DefaultListen || cfg.TLS != false || cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.Port() != 8788 {
		t.Fatalf("Port() = %d", cfg.Port())
	}
}

func TestFileOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
listen = "0.0.0.0:9000"
tls = false
log_level = "debug"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "0.0.0.0:9000" || cfg.LogLevel != "debug" {
		t.Fatalf("file overlay wrong: %+v", cfg)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv(EnvName, "")
	t.Setenv(EnvListen, "1.2.3.4:9999")
	cfg, err := load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "1.2.3.4:9999" {
		t.Fatalf("env override wrong: %+v", cfg)
	}
}

func TestMissingFileKeepsDefaults(t *testing.T) {
	t.Setenv(EnvListen, "")
	cfg, err := load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != DefaultListen {
		t.Fatalf("missing file should keep defaults: %+v", cfg)
	}
}
