// Package config loads relay-server configuration (config.toml + env overrides).
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Defaults for every field; file values overlay these.
const (
	DefaultListen   = "127.0.0.1:8788"
	DefaultTLS      = false
	DefaultLogLevel = "info"
)

// EnvName is the environment variable that points at the config file.
const EnvName = "RELAY_CONFIG"

// EnvListen overrides the listen address (highest precedence).
const EnvListen = "RELAY_LISTEN"

// Config is the relay-server configuration.
type Config struct {
	// Listen is the HTTP listen address ("host:port").
	Listen string `toml:"listen"`
	// TLS enables in-process TLS (R1: always false; production TLS is
	// terminated by Caddy in front).
	TLS bool `toml:"tls"`
	// LogLevel: debug | info | warn | error.
	LogLevel string `toml:"log_level"`
}

// Default returns the dev defaults (loopback listen, no TLS).
func Default() Config {
	return Config{
		Listen:   DefaultListen,
		TLS:      DefaultTLS,
		LogLevel: DefaultLogLevel,
	}
}

// Load reads the config file (path from RELAY_CONFIG, else "config.toml"
// relative to the working directory), overlays it on defaults, then applies
// the RELAY_LISTEN env override.
func Load() (Config, error) {
	return load(os.Getenv(EnvName))
}

func load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = "config.toml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file: keep defaults (do NOT write back — relay-server is
			// a server component, not a user app).
			return applyEnv(cfg), nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
	return applyEnv(cfg), nil
}

func applyEnv(cfg Config) Config {
	if v := os.Getenv(EnvListen); v != "" {
		cfg.Listen = v
	}
	return cfg
}

// SlogLevel maps LogLevel to a slog.Level (unknown → info).
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

// Port extracts the numeric port from Listen (0 on parse failure).
func (c Config) Port() int {
	s := c.Listen
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			p, err := strconv.Atoi(s[i+1:])
			if err != nil {
				return 0
			}
			return p
		}
	}
	return 0
}
