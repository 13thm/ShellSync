// Package lifecycle manages the daemon process lifecycle.
//
// Responsibilities (design §3.1):
//   - single-instance enforcement via a lock file (pid/port/token)
//   - OS signal handling (SIGINT/SIGTERM) for graceful shutdown
//   - cleanup of resources (HTTP server, PTY sessions, log flush) on exit
//
// Implemented in M1-3.
package lifecycle
