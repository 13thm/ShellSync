package logstore

import (
	"encoding/json"
	"time"
)

// Aggregator collects raw bytes for one (terminal, direction) and flushes them
// to the store as sequence-numbered chunks, coalescing bursts by time and size.
//
// Flushing happens when either:
//   - the accumulated buffer reaches MaxChunkBytes (flushed in MaxChunkBytes
//     slices), or
//   - no size threshold is reached within FlushWindow of the first byte of the
//     current burst.
type Aggregator struct {
	manager    *Manager
	terminalID string
	direction  string
	state      *termState
	maxChunk   int
	window     time.Duration

	in   chan []byte
	done chan struct{}
}

func newAggregator(m *Manager, terminalID, direction string, st *termState) *Aggregator {
	a := &Aggregator{
		manager:    m,
		terminalID: terminalID,
		direction:  direction,
		state:      st,
		maxChunk:   m.cfg.MaxChunkBytes,
		window:     m.cfg.FlushWindow,
		in:         make(chan []byte, 256),
		done:       make(chan struct{}),
	}
	go a.run()
	return a
}

// append enqueues a copy of data for aggregation. Blocks if the aggregator is
// saturated; returns the context error if the manager is shutting down.
func (a *Aggregator) append(data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case a.in <- cp:
		return nil
	case <-a.manager.ctx.Done():
		return a.manager.ctx.Err()
	}
}

// stop signals the aggregator to flush remaining bytes and exit, then waits.
func (a *Aggregator) stop() {
	close(a.in)
	<-a.done
}

// flush persists one chunk under a freshly allocated sequence number. For
// stdout it first emits any pending resize marker, guaranteeing the stream
// records the grid change *before* the output produced at the new size —
// clients replaying history re-grid their emulator at exactly that point.
func (a *Aggregator) flush(data []byte) {
	if a.direction == "stdout" {
		if cols, rows, ok := a.manager.takePendingResize(a.terminalID); ok {
			if b, err := json.Marshal(map[string]int{"cols": cols, "rows": rows}); err == nil {
				a.manager.flushChunk(a.terminalID, "resize", a.state.seq.Add(1), b)
			}
		}
	}
	seq := a.state.seq.Add(1)
	a.manager.flushChunk(a.terminalID, a.direction, seq, data)
}

func (a *Aggregator) run() {
	defer close(a.done)

	var buf []byte
	var timer *time.Timer
	var timerC <-chan time.Time

	flush := func() {
		if len(buf) == 0 {
			return
		}
		a.flush(buf)
		buf = nil
	}

	for {
		select {
		case data, ok := <-a.in:
			if !ok {
				flush()
				return
			}
			buf = append(buf, data...)
			// flush whole maxChunk-sized slices immediately
			for len(buf) >= a.maxChunk {
				a.flush(buf[:a.maxChunk])
				buf = buf[a.maxChunk:]
			}
			if len(buf) > 0 {
				if timer == nil {
					timer = time.NewTimer(a.window)
					timerC = timer.C
				}
			} else if timer != nil {
				// buffer drained by size flushes; cancel pending timer.
				// Safe to abandon timerC without draining.
				timer.Stop()
				timer = nil
				timerC = nil
			}
		case <-timerC:
			flush()
			timer = nil
			timerC = nil
		}
	}
}
