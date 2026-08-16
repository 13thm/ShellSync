package ws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/terminal"
)

// Conn is one WebSocket client connection.
type Conn struct {
	hub    *Hub
	c      *websocket.Conn
	userID string

	send     chan []byte
	sendDone chan struct{}

	mu     sync.Mutex
	subs   map[string]*termSub // terminalID -> active subscription
	mirror map[string]func()   // terminalID -> mirror-loop cancel
	closed bool
}

type termSub struct {
	output func()
	status func()
}

func newConn(h *Hub, c *websocket.Conn, userID string) *Conn {
	return &Conn{
		hub: h, c: c, userID: userID,
		send:     make(chan []byte, 1024),
		sendDone: make(chan struct{}),
		subs:     map[string]*termSub{},
		mirror:   map[string]func(){},
	}
}

// run starts the read and write loops and blocks until the connection ends.
func (cn *Conn) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		cn.writePump(ctx)
	}()

	go cn.heartbeat(ctx)

	defer func() {
		cn.close(context.Background())
		cn.hub.unregister(cn)
	}()

	for {
		_, data, err := cn.c.Read(ctx)
		if err != nil {
			return
		}
		var msg rawMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			cn.sendError("", "bad_request", "invalid json")
			continue
		}
		if !cn.dispatch(ctx, msg) {
			return // close requested
		}
	}
}

// dispatch handles one inbound message. Returns false to close the connection.
func (cn *Conn) dispatch(ctx context.Context, msg rawMsg) bool {
	switch msg.Type {
	case "ping":
		cn.sendJSON(envelope{Type: "pong", Ref: msg.ID, OK: true})
	case "terminal.subscribe":
		var p struct {
			TerminalID string `json:"terminalId"`
			FromSeq    int64  `json:"fromSeq"`
			Limit      int    `json:"limit"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			cn.sendError(msg.ID, "bad_request", err.Error())
			return true
		}
		cn.handleSubscribe(ctx, msg.ID, p.TerminalID, p.FromSeq, p.Limit)
	case "terminal.unsubscribe":
		var p struct {
			TerminalID string `json:"terminalId"`
		}
		json.Unmarshal(msg.Payload, &p)
		cn.unsubscribe(p.TerminalID)
		cn.sendJSON(envelope{Type: "terminal.unsubscribed", Ref: msg.ID, OK: true, Payload: map[string]string{"terminalId": p.TerminalID}})
	case "terminal.input":
		var p struct {
			TerminalID string `json:"terminalId"`
			DataB64    string `json:"dataB64"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			cn.sendError(msg.ID, "bad_request", err.Error())
			return true
		}
		cn.handleInput(msg.ID, p.TerminalID, p.DataB64)
	case "terminal.resize":
		var p struct {
			TerminalID string `json:"terminalId"`
			Cols       int    `json:"cols"`
			Rows       int    `json:"rows"`
		}
		json.Unmarshal(msg.Payload, &p)
		cn.handleResize(msg.ID, p.TerminalID, p.Cols, p.Rows)
	case "terminal.history.fetch":
		var p struct {
			TerminalID string `json:"terminalId"`
			FromSeq    int64  `json:"fromSeq"`
			Limit      int    `json:"limit"`
		}
		json.Unmarshal(msg.Payload, &p)
		cn.handleHistory(ctx, msg.ID, p.TerminalID, p.FromSeq, p.Limit)
	case "terminal.mirror":
		var p struct {
			TerminalID string `json:"terminalId"`
			On         bool   `json:"on"`
		}
		json.Unmarshal(msg.Payload, &p)
		cn.handleMirror(msg.ID, p.TerminalID, p.On)
	default:
		cn.sendError(msg.ID, "unknown_type", "unknown event type: "+msg.Type)
	}
	return true
}

func (cn *Conn) handleSubscribe(ctx context.Context, ref, terminalID string, fromSeq int64, limit int) {
	if limit <= 0 {
		limit = 500
	}
	// replace any existing subscription
	cn.unsubscribe(terminalID)

	sess, alive := cn.hub.terminals.Session(terminalID)
	if !alive {
		// exited terminal: replay the FULL history so late viewers still see
		// everything (paging until hasMore is exhausted).
		cn.hub.logMgr.FlushResizeMarker(terminalID) // in case a resize was never followed by output
		cn.replayHistory(ctx, terminalID, fromSeq, limit)
		cn.sendJSON(envelope{Type: "terminal.status", Payload: map[string]any{
			"terminalId": terminalID, "status": "exited",
		}})
		cn.sendJSON(envelope{Type: "terminal.subscribed", Ref: ref, OK: true, Payload: map[string]any{"terminalId": terminalID, "live": false}})
		return
	}

	// report the current PTY size so clients (mobile) can align their local
	// rendering grid with the server-side grid (important for TUI apps).
	if cols, rows := sess.Size(); cols > 0 && rows > 0 {
		cn.sendJSON(envelope{Type: "terminal.size", Payload: map[string]any{
			"terminalId": terminalID, "cols": cols, "rows": rows,
		}})
	}

	// Subscribe FIRST, but buffer live chunks while the history is being
	// replayed. This closes the race between the DB read and the live stream:
	// chunks flushed during the replay are queued and sent afterwards (skipping
	// any seq already covered by the replay), so nothing is lost or duplicated.
	var replayMu sync.Mutex
	replaying := true
	var pending []logstore.FlushedChunk
	outCancel := sess.SubscribeOutput(func(c logstore.FlushedChunk) {
		if c.Direction != "stdout" {
			return // stdin is recorded for audit only; the PTY already echoes it
		}
		replayMu.Lock()
		if replaying {
			pending = append(pending, c)
			replayMu.Unlock()
			return
		}
		replayMu.Unlock()
		cn.sendJSON(envelope{Type: "terminal.output", Payload: map[string]any{
			"terminalId": terminalID,
			"seq":        c.Seq,
			"direction":  c.Direction,
			"contentB64": c.ContentB64,
			"createdAt":  c.CreatedAt,
		}})
	})
	statCancel := sess.SubscribeStatus(func(status string, exitCode *int) {
		payload := map[string]any{"terminalId": terminalID, "status": status}
		if exitCode != nil {
			payload["exitCode"] = *exitCode
		}
		cn.sendJSON(envelope{Type: "terminal.status", Payload: payload})
	})

	cn.mu.Lock()
	cn.subs[terminalID] = &termSub{output: outCancel, status: statCancel}
	cn.mu.Unlock()

	// Flush any pending resize marker so the replayed stream ends at the
	// terminal's current grid (a resize not yet followed by output would
	// otherwise be missing from history and live continuation would render
	// at a stale size).
	cn.hub.logMgr.FlushResizeMarker(terminalID)

	// replay the full history (all pages), then release the buffered live
	// chunks that the replay did not already cover.
	lastSeq := cn.replayHistory(ctx, terminalID, fromSeq, limit)
	replayMu.Lock()
	replaying = false
	buf := pending
	pending = nil
	replayMu.Unlock()
	for _, c := range buf {
		if c.Seq <= lastSeq {
			continue // already replayed from the DB
		}
		cn.sendJSON(envelope{Type: "terminal.output", Payload: map[string]any{
			"terminalId": terminalID,
			"seq":        c.Seq,
			"direction":  c.Direction,
			"contentB64": c.ContentB64,
			"createdAt":  c.CreatedAt,
		}})
	}

	cn.sendJSON(envelope{Type: "terminal.subscribed", Ref: ref, OK: true, Payload: map[string]any{"terminalId": terminalID, "live": true}})
}

func (cn *Conn) handleInput(ref, terminalID, dataB64 string) {
	sess, alive := cn.hub.terminals.Session(terminalID)
	if !alive {
		cn.sendError(ref, "not_running", "terminal not running")
		return
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		cn.sendError(ref, "bad_request", "invalid base64")
		return
	}
	if err := sess.Write(data); err != nil {
		cn.sendError(ref, "io_error", err.Error())
		return
	}
}

func (cn *Conn) handleResize(ref, terminalID string, cols, rows int) {
	sess, alive := cn.hub.terminals.Session(terminalID)
	if !alive {
		cn.sendError(ref, "not_running", "terminal not running")
		return
	}
	// reject degenerate grids: clients report intermediate sizes while their
	// layout animates (e.g. a phone's expanded input panel shrinking the
	// terminal view to a couple of rows); resizing the shared PTY to those
	// wrecks the view on every attached client. 10x3 is the floor.
	if cols < 10 || rows < 3 {
		return
	}
	// skip no-op resizes to avoid broadcast loops between clients
	if curCols, curRows := sess.Size(); curCols == cols && curRows == rows {
		return
	}
	if err := sess.Resize(cols, rows); err != nil {
		cn.sendError(ref, "io_error", err.Error())
		return
	}
	// broadcast the new size to every subscriber so passive viewers
	// (mobile) can re-align their local grid with the PTY.
	cn.hub.Broadcast("terminal.size", map[string]any{
		"terminalId": terminalID, "cols": cols, "rows": rows,
	})
}

// handleMirror toggles mirror mode: the server pushes rendered screen
// snapshots (plain lines) to this client, throttled, whenever output changes.
// This is what mobile clients use instead of replaying raw escape sequences.
func (cn *Conn) handleMirror(ref, terminalID string, on bool) {
	// stop any existing mirror loop
	cn.stopMirror(terminalID)
	if !on {
		cn.sendJSON(envelope{Type: "terminal.mirror", Ref: ref, OK: true, Payload: map[string]any{"terminalId": terminalID, "on": false}})
		return
	}
	sess, alive := cn.hub.terminals.Session(terminalID)
	if !alive {
		cn.sendError(ref, "not_running", "terminal not running")
		return
	}
	scr := sess.Screen()
	if scr == nil {
		cn.sendError(ref, "unavailable", "screen mirror unavailable")
		return
	}

	// immediate snapshot
	cn.sendScreen(terminalID, scr)

	stop := make(chan struct{})
	cn.mu.Lock()
	if cn.closed {
		cn.mu.Unlock()
		close(stop)
		return
	}
	cn.mirror[terminalID] = func() { close(stop) }
	cn.mu.Unlock()

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-cn.sendDone:
				return
			case <-ticker.C:
				if scr.IsDirty() {
					cn.sendScreen(terminalID, scr)
				}
			}
		}
	}()

	cn.sendJSON(envelope{Type: "terminal.mirror", Ref: ref, OK: true, Payload: map[string]any{"terminalId": terminalID, "on": true}})
}

func (cn *Conn) sendScreen(terminalID string, scr *terminal.Screen) {
	lines, cols, rows := scr.Snapshot()
	cn.sendJSON(envelope{Type: "terminal.screen", Payload: map[string]any{
		"terminalId": terminalID,
		"lines":      lines,
		"cols":       cols,
		"rows":       rows,
	}})
}

func (cn *Conn) stopMirror(terminalID string) {
	cn.mu.Lock()
	cancel := cn.mirror[terminalID]
	delete(cn.mirror, terminalID)
	cn.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// unsubscribe stops the mirror loop too when a client unsubscribes.

func (cn *Conn) handleHistory(ctx context.Context, ref, terminalID string, fromSeq int64, limit int) {
	if limit <= 0 {
		limit = 500
	}
	cn.sendHistory(ctx, terminalID, fromSeq, limit)
}

// replayHistory pages through the terminal's persisted history starting at
// fromSeq until the end (one terminal.history event per page) and returns the
// highest seq covered. Clients used to receive only the first page and never
// paged via hasMore, which made long sessions appear truncated in the middle;
// the server now always drives the full replay itself.
func (cn *Conn) replayHistory(ctx context.Context, terminalID string, fromSeq int64, limit int) int64 {
	if limit <= 0 {
		limit = 500
	}
	last := fromSeq - 1
	for {
		chs, hasMore, err := cn.hub.logMgr.ReadRange(ctx, terminalID, fromSeq, limit)
		if err != nil {
			cn.sendError("", "io_error", err.Error())
			return last
		}
		out := make([]map[string]any, 0, len(chs))
		var toSeq int64
		for _, c := range chs {
			toSeq = c.Seq
			if c.Direction != "stdout" && c.Direction != "resize" {
				continue // replay renders what the terminal displayed; resize markers
				// re-grid the client emulator at the point they occurred
			}
			out = append(out, map[string]any{
				"seq":        c.Seq,
				"direction":  c.Direction,
				"contentB64": c.ContentB64,
				"createdAt":  c.CreatedAt,
			})
		}
		cn.sendJSON(envelope{Type: "terminal.history", Payload: map[string]any{
			"terminalId": terminalID,
			"fromSeq":    fromSeq,
			"toSeq":      toSeq,
			"hasMore":    hasMore,
			"chunks":     out,
		}})
		if len(chs) == 0 || !hasMore {
			return last
		}
		last = toSeq
		fromSeq = toSeq + 1
	}
}

// sendHistory sends a terminal.history event with a single page of chunks
// (used by the explicit terminal.history.fetch paging API).
func (cn *Conn) sendHistory(ctx context.Context, terminalID string, fromSeq int64, limit int) {
	chs, hasMore, err := cn.hub.logMgr.ReadRange(ctx, terminalID, fromSeq, limit)
	if err != nil {
		cn.sendError("", "io_error", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(chs))
	var toSeq int64
	for _, c := range chs {
		toSeq = c.Seq
		if c.Direction != "stdout" && c.Direction != "resize" {
			continue // replay renders what the terminal displayed; resize markers
			// re-grid the client emulator at the point they occurred
		}
		out = append(out, map[string]any{
			"seq":        c.Seq,
			"direction":  c.Direction,
			"contentB64": c.ContentB64,
			"createdAt":  c.CreatedAt,
		})
	}
	cn.sendJSON(envelope{Type: "terminal.history", Payload: map[string]any{
		"terminalId": terminalID,
		"fromSeq":    fromSeq,
		"toSeq":      toSeq,
		"hasMore":    hasMore,
		"chunks":     out,
	}})
}

func (cn *Conn) unsubscribe(terminalID string) {
	cn.stopMirror(terminalID)
	cn.mu.Lock()
	sub := cn.subs[terminalID]
	delete(cn.subs, terminalID)
	cn.mu.Unlock()
	if sub != nil {
		if sub.output != nil {
			sub.output()
		}
		if sub.status != nil {
			sub.status()
		}
	}
}

// writePump drains the send channel to the underlying connection.
func (cn *Conn) writePump(ctx context.Context) {
	defer close(cn.sendDone)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-cn.send:
			if !ok {
				return
			}
			if err := cn.c.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		}
	}
}

// heartbeat sends periodic pings; a failure closes the connection.
func (cn *Conn) heartbeat(ctx context.Context) {
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := cn.c.Ping(pingCtx); err != nil {
				cancel()
				_ = cn.c.Close(websocket.StatusPolicyViolation, "ping timeout")
				return
			}
			cancel()
		}
	}
}

func (cn *Conn) sendJSON(msg envelope) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	cn.trySend(b)
}

func (cn *Conn) sendError(ref, code, message string) {
	cn.sendJSON(envelope{Ref: ref, OK: false, Error: &errBody{Code: code, Message: message}})
}

// trySend pushes bytes to the send channel without blocking; drops on overflow.
func (cn *Conn) trySend(b []byte) {
	select {
	case cn.send <- b:
	default:
		slog.Warn("ws: send buffer full, dropping message")
	}
}

func (cn *Conn) close(ctx context.Context) {
	cn.mu.Lock()
	if cn.closed {
		cn.mu.Unlock()
		return
	}
	cn.closed = true
	subs := cn.subs
	cn.subs = map[string]*termSub{}
	mirrors := cn.mirror
	cn.mirror = map[string]func(){}
	cn.mu.Unlock()

	for _, cancel := range mirrors {
		cancel()
	}
	for _, s := range subs {
		if s.output != nil {
			s.output()
		}
		if s.status != nil {
			s.status()
		}
	}
	_ = cn.c.Close(websocket.StatusNormalClosure, "")
	close(cn.send)
	<-cn.sendDone
}
