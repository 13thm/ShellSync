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
	closed bool
}

type termSub struct {
	output func()
	status func()
}

func newConn(h *Hub, c *websocket.Conn, userID string) *Conn {
	return &Conn{
		hub: h, c: c, userID: userID,
		send:     make(chan []byte, 256),
		sendDone: make(chan struct{}),
		subs:     map[string]*termSub{},
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
	default:
		cn.sendError(msg.ID, "unknown_type", "unknown event type: "+msg.Type)
	}
	return true
}

func (cn *Conn) handleSubscribe(ctx context.Context, ref, terminalID string, fromSeq int64, limit int) {
	if limit <= 0 {
		limit = 500
	}
	// history first
	cn.sendHistory(ctx, terminalID, fromSeq, limit)

	sess, alive := cn.hub.terminals.Session(terminalID)
	if !alive {
		cn.sendJSON(envelope{Type: "terminal.status", Payload: map[string]any{
			"terminalId": terminalID, "status": "exited",
		}})
		cn.sendJSON(envelope{Type: "terminal.subscribed", Ref: ref, OK: true, Payload: map[string]any{"terminalId": terminalID, "live": false}})
		return
	}

	// replace any existing subscription
	cn.unsubscribe(terminalID)

	outCancel := sess.SubscribeOutput(func(c logstore.FlushedChunk) {
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
	if err := sess.Resize(cols, rows); err != nil {
		cn.sendError(ref, "io_error", err.Error())
		return
	}
}

func (cn *Conn) handleHistory(ctx context.Context, ref, terminalID string, fromSeq int64, limit int) {
	if limit <= 0 {
		limit = 500
	}
	cn.sendHistory(ctx, terminalID, fromSeq, limit)
}

// sendHistory sends a terminal.history event with a page of chunks.
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
	cn.mu.Unlock()

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
