// Package sync is the in-process domain-event bus.
//
// Responsibilities (design §3.1, §7.4):
//   - publish entity change events (created/updated/deleted)
//   - allow the WebSocket layer to subscribe and broadcast to clients
//
// Implemented in M2-6.
package sync
