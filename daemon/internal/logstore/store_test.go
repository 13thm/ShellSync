package logstore

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shellsync/daemon/internal/repository"
)

// setupManager returns a manager over a fresh migrated DB with one terminal,
// using a short window (50ms) and small chunk (1KiB) for deterministic tests.
func setupManager(t *testing.T) (*Manager, *repository.LogRepo, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := repository.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := repository.SeedDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	termRepo := repository.NewTerminalRepo(db)
	term, err := termRepo.Create(ctx, repository.TerminalCreate{
		UserID: repository.DefaultUserID, ShellType: "bash",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	logRepo := repository.NewLogRepo(db)
	logsPath := filepath.Join(t.TempDir(), "logs")
	mgr := NewManager(logRepo, Config{
		FlushWindow:   50 * time.Millisecond,
		MaxChunkBytes: 1024,
		LogsPath:      logsPath,
	})
	t.Cleanup(mgr.CloseAll)
	return mgr, logRepo, term.ID, logsPath
}

func countChunks(t *testing.T, repo *repository.LogRepo, termID string) int {
	t.Helper()
	chunks, _, err := repo.ReadRange(context.Background(), termID, 1, 1_000_000)
	if err != nil {
		t.Fatalf("readRange: %v", err)
	}
	return len(chunks)
}

func decodeAll(t *testing.T, repo *repository.LogRepo, termID string) string {
	t.Helper()
	chunks, _, err := repo.ReadRange(context.Background(), termID, 1, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, c := range chunks {
		raw, err := base64.StdEncoding.DecodeString(c.ContentB64)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(raw)
	}
	return sb.String()
}

// Many tiny writes within one window must merge into a single chunk.
func TestAppendAggregatesByTime(t *testing.T) {
	mgr, repo, termID, _ := setupManager(t)
	ctx := context.Background()
	mgr.Register(ctx, termID)

	for i := 0; i < 50; i++ {
		if err := mgr.Append(termID, "stdout", []byte("x")); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	time.Sleep(150 * time.Millisecond) // > window

	if n := countChunks(t, repo, termID); n != 1 {
		t.Fatalf("chunks = %d, want 1 (time aggregation)", n)
	}
	if got := decodeAll(t, repo, termID); got != strings.Repeat("x", 50) {
		t.Fatalf("decoded = %q, want 50 x's", got)
	}
}

// A single large write must split into maxChunk-sized chunks.
func TestAppendAggregatesBySize(t *testing.T) {
	mgr, repo, termID, _ := setupManager(t) // MaxChunkBytes = 1024

	big := make([]byte, 2500) // -> 1024 + 1024 + 452
	if err := mgr.Append(termID, "stdout", big); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)

	if n := countChunks(t, repo, termID); n != 3 {
		t.Fatalf("chunks = %d, want 3 (size split)", n)
	}
	if got := decodeAll(t, repo, termID); len(got) != 2500 {
		t.Fatalf("decoded len = %d, want 2500", len(got))
	}
}

// Sequence numbers are contiguous and start at 1 for a fresh terminal.
func TestSeqContiguous(t *testing.T) {
	mgr, repo, termID, _ := setupManager(t)
	ctx := context.Background()

	for _, s := range []string{"a", "bb", "ccc"} {
		if err := mgr.Append(termID, "stdout", []byte(s)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(90 * time.Millisecond) // separate windows -> separate flushes
	}
	chunks, _, err := repo.ReadRange(ctx, termID, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(chunks))
	}
	for i, c := range chunks {
		if c.Seq != int64(i+1) {
			t.Fatalf("seq[%d] = %d, want %d", i, c.Seq, i+1)
		}
	}
	if got := decodeAll(t, repo, termID); got != "abbccc" {
		t.Fatalf("decoded = %q", got)
	}
}

// OnFlush is invoked for each persisted chunk.
func TestOnFlushHook(t *testing.T) {
	mgr, _, termID, _ := setupManager(t)
	var n int32
	mgr.OnFlush = func(c FlushedChunk) {
		if c.TerminalID != termID || c.Direction != "stdout" || c.RawLen != 2 {
			t.Errorf("unexpected chunk: %+v", c)
		}
		atomic.AddInt32(&n, 1)
	}

	mgr.Append(termID, "stdout", []byte("hi"))
	time.Sleep(90 * time.Millisecond)

	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("onFlush calls = %d, want 1", n)
	}
}

// Register seeds the sequence counter from existing DB rows.
func TestRegisterSeedsFromDB(t *testing.T) {
	mgr, repo, termID, _ := setupManager(t)
	ctx := context.Background()

	// pretend the terminal already had two chunks persisted
	enc := base64.StdEncoding.EncodeToString([]byte("old"))
	repo.AppendChunk(ctx, termID, 1, "stdout", enc, now())
	repo.AppendChunk(ctx, termID, 2, "stdout", enc, now())

	mgr.Register(ctx, termID)
	mgr.Append(termID, "stdout", []byte("new"))
	time.Sleep(90 * time.Millisecond)

	max, _ := mgr.MaxSeq(ctx, termID)
	if max != 3 {
		t.Fatalf("maxSeq = %d, want 3", max)
	}
}

// Close flushes any buffered bytes immediately.
func TestCloseFlushesRemaining(t *testing.T) {
	mgr, repo, termID, _ := setupManager(t)

	mgr.Append(termID, "stdout", []byte("buffered"))
	mgr.Close(termID) // no sleep -> must flush synchronously

	if n := countChunks(t, repo, termID); n != 1 {
		t.Fatalf("chunks after close = %d, want 1", n)
	}
}

// ArchiveOlderThan moves old chunks to a file and deletes them from the DB.
func TestArchiveOlderThan(t *testing.T) {
	mgr, repo, termID, logsPath := setupManager(t)
	ctx := context.Background()

	old := now() - 2*time.Hour.Milliseconds()
	recent := now()
	repo.AppendChunk(ctx, termID, 1, "stdout", base64.StdEncoding.EncodeToString([]byte("old1")), old)
	repo.AppendChunk(ctx, termID, 2, "stdout", base64.StdEncoding.EncodeToString([]byte("old2")), old)
	repo.AppendChunk(ctx, termID, 3, "stdout", base64.StdEncoding.EncodeToString([]byte("new1")), recent)

	n, err := mgr.ArchiveOlderThan(ctx, termID, time.Hour)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 2 {
		t.Fatalf("archived = %d, want 2", n)
	}

	// only the recent chunk remains in DB
	remaining, _, _ := repo.ReadRange(ctx, termID, 1, 100)
	if len(remaining) != 1 || remaining[0].Seq != 3 {
		t.Fatalf("remaining = %+v", remaining)
	}

	// archive file contains decoded old bytes
	data, err := os.ReadFile(filepath.Join(logsPath, termID+".log"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if string(data) != "old1old2" {
		t.Fatalf("archive = %q", string(data))
	}
}
