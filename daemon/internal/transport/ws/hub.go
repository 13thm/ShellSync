package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/shellsync/daemon/internal/auth"
	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/service"
)

// Hub owns all WebSocket connections and fans out domain events to them.
type Hub struct {
	auth      *auth.Verifier
	terminals *service.TerminalService
	logMgr    *logstore.Manager
	bus       *eventbus.Bus

	mu    sync.Mutex
	conns map[*Conn]struct{}
}

// NewHub creates a Hub and starts a goroutine broadcasting bus events.
func NewHub(verifier *auth.Verifier, terminals *service.TerminalService, logMgr *logstore.Manager, bus *eventbus.Bus) *Hub {
	h := &Hub{
		auth: verifier, terminals: terminals, logMgr: logMgr, bus: bus,
		conns: map[*Conn]struct{}{},
	}
	if bus != nil {
		go h.broadcastLoop()
	}
	return h
}

// broadcastLoop forwards domain events to every connection.
func (h *Hub) broadcastLoop() {
	ch, cancel := h.bus.Subscribe()
	defer cancel()
	for ev := range ch {
		msg := envelope{Type: ev.Type, Payload: ev.Payload}
		b, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		h.mu.Lock()
		for c := range h.conns {
			c.trySend(b)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) register(c *Conn) {
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *Conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
}

// ServeHTTP upgrades to a WebSocket after verifying the token query param.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	uid, ok := h.auth.Verify(r.Context(), tok)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow any origin (Desktop/Mobile are local/LAN)
	})
	if err != nil {
		slog.Debug("ws accept", "err", err)
		return
	}
	// reasonable limits for terminal streams
	c.SetReadLimit(1 << 20) // 1 MiB per message

	conn := newConn(h, c, uid)
	h.register(conn)
	conn.run(r.Context())
}

// envelope is the outgoing wire message.
type envelope struct {
	Type    string   `json:"type"`
	Ref     string   `json:"ref,omitempty"`
	OK      bool     `json:"ok,omitempty"`
	Payload any      `json:"payload,omitempty"`
	Error   *errBody `json:"error,omitempty"`
}

// rawMsg is the incoming wire message (payload kept raw for lazy parsing).
type rawMsg struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
