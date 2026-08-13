// Package config provides configuration loading for the ShellSync daemon.
//
// Responsibilities (design §3.1):
//   - read ~/.shellsync/config.json
//   - provide sensible defaults (data dir, ports, log retention, etc.)
//   - create the data directory on first run
//
// Implemented in M1-2.
package config
