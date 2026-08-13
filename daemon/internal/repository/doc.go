// Package repository is the SQLite data-access layer.
//
// Responsibilities (design §3.1):
//   - open the database with WAL mode and foreign keys
//   - run idempotent schema migrations (see migrations/)
//   - provide CRUD repos for users/devices/tasks/terminals/terminal_logs/
//     todos/settings/sync_cursors
//
// Implemented in M1-4 (db + migrations) and M1-5 (repos).
package repository
