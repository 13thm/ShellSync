package logstore

import (
	"context"
	"encoding/base64"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shellsync/daemon/internal/repository"
)

// Config tunes the log store.
type Config struct {
	// MaxChunkBytes is the largest single persisted chunk (default 16 KiB).
	MaxChunkBytes int
	// FlushWindow is how long bytes accumulate before being flushed even when
	// the size threshold has not been reached (default 16ms).
	FlushWindow time.Duration
	// LogsPath is the directory used for cold-archived log files (empty
	// disables archiving).
	LogsPath string
}

// FlushedChunk describes a chunk that was just persisted to the database.
type FlushedChunk struct {
	TerminalID string
	Seq        int64
	Direction  string
	ContentB64 string
	RawLen     int
	CreatedAt  int64
}

// Manager coordinates per-terminal byte aggregation, sequence numbering and
// history access for terminal logs.
type Manager struct {
	repo *repository.LogRepo
	cfg  Config

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	states map[string]*termState

	// OnFlush, if set, is invoked from an aggregator goroutine whenever a
	// chunk is persisted. Set it once right after NewManager. This is the hook
	// the terminal manager uses to broadcast new output to subscribers.
	OnFlush func(FlushedChunk)
}

// termState holds the per-terminal sequence counter and its aggregators.
type termState struct {
	seq  atomic.Int64
	mu   sync.Mutex
	aggs map[string]*Aggregator
}

// NewManager creates a Manager.
func NewManager(repo *repository.LogRepo, cfg Config) *Manager {
	if cfg.MaxChunkBytes <= 0 {
		cfg.MaxChunkBytes = 16 * 1024
	}
	if cfg.FlushWindow <= 0 {
		cfg.FlushWindow = 16 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		repo:   repo,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		states: map[string]*termState{},
	}
}

func now() int64 { return time.Now().UnixMilli() }

// Register initializes tracking for a terminal, seeding its sequence counter
// from the database MAX(seq). Idempotent and safe to call before any Append.
func (m *Manager) Register(ctx context.Context, terminalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.states[terminalID]; ok {
		return nil
	}
	max, err := m.repo.MaxSeq(ctx, terminalID)
	if err != nil {
		return err
	}
	st := &termState{aggs: map[string]*Aggregator{}}
	st.seq.Store(max) // nextSeq = Add(1) -> max+1
	m.states[terminalID] = st
	return nil
}

// Append enqueues raw bytes for a terminal/direction. Bytes are aggregated and
// persisted asynchronously; this returns once the bytes are accepted. Empty
// data is ignored.
func (m *Manager) Append(terminalID, direction string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	agg, err := m.aggregator(terminalID, direction)
	if err != nil {
		return err
	}
	return agg.append(data)
}

// aggregator returns the aggregator for (terminalID, direction), creating and
// registering the terminal if needed.
func (m *Manager) aggregator(terminalID, direction string) (*Aggregator, error) {
	if err := m.Register(m.ctx, terminalID); err != nil {
		return nil, err
	}
	m.mu.Lock()
	st := m.states[terminalID]
	m.mu.Unlock()

	st.mu.Lock()
	defer st.mu.Unlock()
	agg, ok := st.aggs[direction]
	if !ok {
		agg = newAggregator(m, terminalID, direction, st)
		st.aggs[direction] = agg
	}
	return agg, nil
}

// ReadRange returns up to limit chunks with seq >= fromSeq plus a hasMore flag.
func (m *Manager) ReadRange(ctx context.Context, terminalID string, fromSeq int64, limit int) ([]repository.TerminalLog, bool, error) {
	return m.repo.ReadRange(ctx, terminalID, fromSeq, limit)
}

// Tail returns the last limit chunks in ascending order.
func (m *Manager) Tail(ctx context.Context, terminalID string, limit int) ([]repository.TerminalLog, error) {
	return m.repo.Tail(ctx, terminalID, limit)
}

// MaxSeq returns the highest persisted seq for a terminal (0 if none).
func (m *Manager) MaxSeq(ctx context.Context, terminalID string) (int64, error) {
	return m.repo.MaxSeq(ctx, terminalID)
}

// Close flushes and stops all aggregators for a terminal.
func (m *Manager) Close(terminalID string) {
	m.mu.Lock()
	st := m.states[terminalID]
	delete(m.states, terminalID)
	m.mu.Unlock()
	if st == nil {
		return
	}
	st.mu.Lock()
	aggs := make([]*Aggregator, 0, len(st.aggs))
	for _, a := range st.aggs {
		aggs = append(aggs, a)
	}
	st.mu.Unlock()
	for _, a := range aggs {
		a.stop()
	}
}

// CloseAll flushes and stops everything. Call on daemon shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.states))
	for id := range m.states {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Close(id)
	}
	m.cancel()
}

// flushChunk persists one aggregated chunk. Called from aggregator goroutines.
func (m *Manager) flushChunk(terminalID, direction string, seq int64, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	ts := now()
	if err := m.repo.AppendChunk(m.ctx, terminalID, seq, direction, encoded, ts); err != nil {
		slog.Error("logstore: persist chunk", "terminal", terminalID, "seq", seq, "err", err)
		return
	}
	if m.OnFlush != nil {
		m.OnFlush(FlushedChunk{
			TerminalID: terminalID,
			Seq:        seq,
			Direction:  direction,
			ContentB64: encoded,
			RawLen:     len(data),
			CreatedAt:  ts,
		})
	}
}
