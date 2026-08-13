// Package service implements domain logic on top of the repository.
//
// Responsibilities (design §3.1):
//   - task/todo/terminal creation, update and state-machine transitions
//   - cascade rules (e.g. deleting a task unbinds its terminals)
//   - emit domain events to the sync bus
//
// Implemented in M2-1.
package service
