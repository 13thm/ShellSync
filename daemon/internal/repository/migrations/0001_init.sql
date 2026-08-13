-- ShellSync initial schema (see 系统设计说明书 §4.2)
-- All timestamps are Unix milliseconds (INTEGER). Booleans are INTEGER 0/1.

-- accounts (MVP: a single seeded local user)
CREATE TABLE users (
  id           TEXT PRIMARY KEY,
  username     TEXT NOT NULL UNIQUE,
  display_name TEXT,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);

-- paired devices (desktop + mobile)
CREATE TABLE devices (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,           -- desktop | ios | android
  session_token TEXT NOT NULL UNIQUE,    -- bcrypt-hashed
  last_seen_at  INTEGER,
  created_at    INTEGER NOT NULL,
  revoked       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_devices_user ON devices(user_id);

-- tasks (业务任务)
CREATE TABLE tasks (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  description  TEXT,
  status       TEXT NOT NULL DEFAULT 'pending', -- pending|running|paused|done
  color        TEXT,
  archived     INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX idx_tasks_user_status ON tasks(user_id, archived, status);
CREATE INDEX idx_tasks_updated     ON tasks(updated_at);

-- terminals (终端实例)
CREATE TABLE terminals (
  id             TEXT PRIMARY KEY,
  user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  task_id        TEXT REFERENCES tasks(id) ON DELETE SET NULL, -- nullable
  name           TEXT NOT NULL,
  shell_type     TEXT NOT NULL,          -- cmd|powershell|bash|zsh
  cwd            TEXT,
  cols           INTEGER NOT NULL DEFAULT 80,
  rows           INTEGER NOT NULL DEFAULT 24,
  env            TEXT,                   -- JSON string of extra env vars
  status         TEXT NOT NULL DEFAULT 'running', -- running|exited|crashed
  exit_code      INTEGER,
  os_pid         INTEGER,
  last_seq       INTEGER NOT NULL DEFAULT 0,
  created_at     INTEGER NOT NULL,
  last_active_at INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);
CREATE INDEX idx_terminals_task    ON terminals(task_id);
CREATE INDEX idx_terminals_status  ON terminals(status);
CREATE INDEX idx_terminals_updated ON terminals(updated_at);

-- terminal logs (terminal I/O chunks; high write volume)
CREATE TABLE terminal_logs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  terminal_id  TEXT NOT NULL REFERENCES terminals(id) ON DELETE CASCADE,
  seq          INTEGER NOT NULL,         -- monotonic per terminal, from 1
  direction    TEXT NOT NULL,            -- stdout|stderr|stdin|system
  content_b64  TEXT NOT NULL,            -- base64(raw bytes), <=64KB advised
  created_at   INTEGER NOT NULL,
  UNIQUE(terminal_id, seq)
);
CREATE INDEX idx_logs_term_seq ON terminal_logs(terminal_id, seq);

-- todos (待办)
CREATE TABLE todos (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  task_id      TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  terminal_id  TEXT REFERENCES terminals(id) ON DELETE SET NULL,
  title        TEXT NOT NULL,
  content      TEXT,
  status       TEXT NOT NULL DEFAULT 'pending', -- pending|done
  priority     INTEGER NOT NULL DEFAULT 0,      -- 0 normal|1 important|2 urgent
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX idx_todos_task    ON todos(task_id);
CREATE INDEX idx_todos_status  ON todos(status, sort_order);
CREATE INDEX idx_todos_updated ON todos(updated_at);

-- global settings (KV)
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL -- JSON string
);

-- per-device sync cursors (incremental sync, optional)
CREATE TABLE sync_cursors (
  device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
  entity    TEXT NOT NULL,   -- task|terminal|todo
  last_ts   INTEGER NOT NULL
);
