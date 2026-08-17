package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// helloTimeout bounds the wait for the mandatory first hello frame.
const helloTimeout = 10 * time.Second

// ServeWS is the /ws WebSocket endpoint: it upgrades the connection,
// validates the mandatory hello{role, ver} first frame, then hands the
// session to the hub. Mount it on any http mux/router.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Non-browser clients (daemon, mobile app) send no Origin; the relay
		// is not browser-reachable by design.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	log := h.log

	sess := NewSession(conn, r.RemoteAddr, log)

	// --- hello handshake ---
	ctx, cancel := context.WithTimeout(r.Context(), helloTimeout)
	first, err := readControlFrame(ctx, conn)
	cancel()
	if err != nil || first.T != THello {
		h.reject(sess, "first frame must be hello")
		return
	}
	if first.Role != RoleDaemon && first.Role != RoleClient {
		h.reject(sess, "bad role")
		return
	}
	if first.Ver > ProtocolVersion {
		h.reject(sess, "unsupported protocol version")
		return
	}
	sess.SetRole(first.Role, "")
	_ = sess.WriteControl(context.Background(), Frame{T: THello, Ver: ProtocolVersion})

	h.Attach(sess)
	if err := sess.Run(r.Context()); err != nil {
		log.Debug("session ended", "err", err, "role", first.Role)
	}
}

func (h *Hub) reject(sess *Session, why string) {
	_ = sess.WriteControl(context.Background(), Frame{T: TError, Code: ErrProtocol, Why: why})
	sess.Close()
}

// readControlFrame reads exactly one text message and decodes it.
func readControlFrame(ctx context.Context, conn *websocket.Conn) (Frame, error) {
	msgType, reader, err := conn.Reader(ctx)
	if err != nil {
		return Frame{}, err
	}
	if msgType != websocket.MessageText {
		return Frame{}, errors.New("expected text frame")
	}
	b := make([]byte, 0, 128)
	buf := make([]byte, 1024)
	for {
		n, rerr := reader.Read(buf)
		b = append(b, buf[:n]...)
		if len(b) > maxControlSize {
			return Frame{}, errors.New("control frame too large")
		}
		if rerr != nil {
			break
		}
	}
	return DecodeControl(b)
}

// HealthHandler / MetricsHandler expose simple JSON endpoints so callers can
// mount them without depending on chi.
func (h *Hub) HealthHandler(ver string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "ver": ver})
	}
}

func (h *Hub) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, h.Metrics())
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
