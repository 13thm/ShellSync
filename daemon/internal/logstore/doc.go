// Package logstore persists terminal I/O as ordered, sequence-numbered chunks.
//
// Responsibilities (design §3.1, §4.5):
//   - aggregate raw bytes into chunks (16ms / 16KB window)
//   - assign a monotonically increasing seq per terminal
//   - support range/tail reads for history replay
//   - archive cold data to per-terminal files
//
// Implemented in M1-7.
package logstore
