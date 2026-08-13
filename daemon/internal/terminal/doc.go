// Package terminal manages live terminal sessions.
//
// Responsibilities (design §3.1):
//   - hold a pty instance and a per-terminal seq counter
//   - run a read goroutine that aggregates output into the logstore
//   - broadcast output/status to subscribers (WebSocket layer)
//   - restart-on-crash and metadata recovery on daemon restart
//
// Implemented in M1-8.
package terminal
