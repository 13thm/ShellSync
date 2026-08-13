package logstore

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ArchiveOlderThan moves chunks older than age for a terminal into an
// append-only file under LogsPath (raw decoded bytes) and deletes them from
// the database. Returns the number of chunks archived.
//
// Cold-file reading is not wired into ReadRange yet; archived history is
// preserved on disk for future retrieval (see design §4.5).
func (m *Manager) ArchiveOlderThan(ctx context.Context, terminalID string, age time.Duration) (int, error) {
	if m.cfg.LogsPath == "" {
		return 0, errors.New("logstore: LogsPath not configured")
	}
	if age <= 0 {
		return 0, errors.New("logstore: age must be positive")
	}
	cutoff := now() - age.Milliseconds()

	rows, err := m.repo.ListBefore(ctx, terminalID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("list old chunks: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	if err := os.MkdirAll(m.cfg.LogsPath, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir logs: %w", err)
	}
	path := filepath.Join(m.cfg.LogsPath, terminalID+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open archive file: %w", err)
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	for _, r := range rows {
		raw, err := base64.StdEncoding.DecodeString(r.ContentB64)
		if err != nil {
			return 0, fmt.Errorf("decode chunk seq=%d: %w", r.Seq, err)
		}
		if _, err := bw.Write(raw); err != nil {
			return 0, fmt.Errorf("write archive: %w", err)
		}
	}
	if err := bw.Flush(); err != nil {
		return 0, err
	}

	n, err := m.repo.DeleteBefore(ctx, terminalID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete archived chunks: %w", err)
	}
	return int(n), nil
}
