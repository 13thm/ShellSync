// Package config provides configuration loading for the ShellSync daemon.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Filesystem layout inside the data directory.
const (
	AppName       = "shellsync"
	ConfigFile    = "config.json"
	DBFile        = "shellsync.db"
	LogsDir       = "logs"
	ConfigVersion = 1
)

// EnvDataDir, when set, overrides the data directory location (handy for tests
// and for users who want data on another drive).
const EnvDataDir = "SHELLSYNC_DATA_DIR"

// Config holds all daemon configuration.
type Config struct {
	// Version is the config schema version (for future migrations).
	Version int `json:"version"`
	// DataDir is the root directory for all persistent data.
	DataDir string `json:"dataDir"`
	// HTTPPort is the REST/WS listen port. 0 means pick a free dynamic port.
	HTTPPort int `json:"httpPort"`
	// WSPort is the WebSocket listen port. 0 means reuse HTTPPort.
	WSPort int `json:"wsPort"`
	// LogLevel is one of: debug | info | warn | error.
	LogLevel string `json:"logLevel"`
	// LogRetention is how many days terminal logs stay in the hot SQLite store
	// before being archived to files. 0 means keep forever.
	LogRetention int `json:"logRetention"`
}

// Default returns the default configuration.
func Default() Config {
	return Config{
		Version:      ConfigVersion,
		DataDir:      defaultDataDir(),
		HTTPPort:     0, // dynamic
		WSPort:       0, // reuse http port
		LogLevel:     "info",
		LogRetention: 7,
	}
}

func defaultDataDir() string {
	if v := os.Getenv(EnvDataDir); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// last-resort fallback: a relative directory
		return filepath.Join(".", AppName)
	}
	return filepath.Join(home, "."+AppName)
}

// Load reads the config from its default location (~/.shellsync/config.json).
// If the file does not exist, defaults are used, the data directory tree is
// created, and the config is written back so the user can inspect/edit it.
func Load() (Config, error) {
	cfg := Default()
	return loadOrCreate(cfg, cfg.FilePath())
}

// loadOrCreate reads path if present (overlaying defaults), otherwise persists
// defaults to path.
func loadOrCreate(cfg Config, path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return cfg, fmt.Errorf("read config %s: %w", path, err)
		}
		// Missing file: persist defaults so the location is discoverable.
		if err := cfg.EnsureDirs(); err != nil {
			return cfg, err
		}
		if err := cfg.WriteFile(path); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	// Overlay file values on top of defaults so missing fields keep defaults.
	loaded := cfg
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if loaded.DataDir == "" {
		loaded.DataDir = cfg.DataDir
	}
	if err := loaded.EnsureDirs(); err != nil {
		return loaded, err
	}
	return loaded, nil
}

// FilePath returns the absolute path to config.json inside DataDir.
func (c Config) FilePath() string { return filepath.Join(c.DataDir, ConfigFile) }

// DBPath returns the absolute path to the SQLite database file.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, DBFile) }

// LogsPath returns the directory used for archived terminal logs.
func (c Config) LogsPath() string { return filepath.Join(c.DataDir, LogsDir) }

// EnsureDirs creates the data directory and the logs sub-directory.
func (c Config) EnsureDirs() error {
	for _, dir := range []string{c.DataDir, c.LogsPath()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	return nil
}

// WriteFile writes the config as pretty-printed JSON to path.
func (c Config) WriteFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SlogLevel maps LogLevel to a slog.Level (unknown values default to info).
func (c Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
