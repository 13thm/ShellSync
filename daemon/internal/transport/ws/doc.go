// Package ws exposes the daemon's real-time WebSocket server.
//
// Responsibilities (design §3.1, §5):
//   - connection hub managing subscriptions per terminal
//   - terminal stream events (subscribe/input/output/resize/history)
//   - data-sync event fan-out
//   - heartbeat / ping-pong
//
// Implemented in M2-5.
package ws
