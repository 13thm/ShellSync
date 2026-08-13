// Package http exposes the daemon's REST API.
//
// Responsibilities (design §3.1, §6):
//   - chi router with CORS / recovery / logging / auth middleware
//   - resource handlers: health, tasks, terminals, todos, logs,
//     settings, pair, devices, system
//   - unified {code,data,message} response envelope
//
// Implemented in M2-2.
package http
