package relay

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Pairing-code policy (doc §6.2).
const (
	maxCodeLen  = 16
	maxCodeTTL  = 10 * time.Minute
	maxDevIDLen = 64
	maxSignLen  = 256
)

// Concurrency caps guard the relay against stream exhaustion.
const (
	maxStreamsPerClient = 16
	maxStreamsPerDaemon = 64
)

// Error codes sent in {"t":"error","code":…} frames.
const (
	ErrRateLimited   = "rate_limited"
	ErrInvalidCode   = "invalid_code"
	ErrDaemonOffline = "daemon_offline"
	ErrProtocol      = "protocol"
	ErrTooManyStream = "too_many_streams"
)

// Metrics is a snapshot of relay counters (exposed at /metrics).
type Metrics struct {
	Conns    int64 `json:"connections"`
	Daemons  int64 `json:"daemons"`
	Streams  int64 `json:"streams"`
	CodeReg  int64 `json:"codes_registered"`
	Claims   int64 `json:"claims"`
	ClaimHit int64 `json:"claim_hits"`
	Relayed  int64 `json:"relayed_bytes"`
}

type codeEntry struct {
	devID     string
	expiresAt time.Time
}

type pipe struct {
	id     uint32
	client *Session
	daemon *Session
	devID  string
}

// Hub is the in-memory relay state: registered daemons, live pairing codes,
// claimed client bindings and active streams. No persistence — restarting
// the relay simply drops all state (clients reconnect, codes are re-issued).
type Hub struct {
	mu  sync.Mutex
	now func() time.Time
	log *slog.Logger

	daemons  map[string]*Session   // devId → daemon session
	codes    map[string]codeEntry  // pairing code → {devId, expiry}
	claimed  map[*Session]string   // client session → devId (after claim)
	streams  map[uint32]*pipe      // streamId → pipe
	bySess   map[*Session][]uint32 // session → its stream ids
	nextID   uint32
	limiters map[string]*ipLimiter // client IP → claim limiter

	m Metrics
}

// NewHub creates a Hub. now defaults to time.Now (overridable in tests).
func NewHub(log *slog.Logger, now func() time.Time) *Hub {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		now:      now,
		log:      log,
		daemons:  map[string]*Session{},
		codes:    map[string]codeEntry{},
		claimed:  map[*Session]string{},
		streams:  map[uint32]*pipe{},
		bySess:   map[*Session][]uint32{},
		limiters: map[string]*ipLimiter{},
	}
}

// Metrics returns a snapshot of the counters.
func (h *Hub) Metrics() Metrics {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.m
	m.Daemons = int64(len(h.daemons))
	m.Streams = int64(len(h.streams))
	return m
}

// Attach installs hub handlers on a session and counts the connection.
// Called right after the hello frame is validated by the endpoint code.
func (h *Hub) Attach(s *Session) {
	s.Handlers(h.onControl, h.onData)
	s.OnClose(h.onSessionClose)
	h.mu.Lock()
	h.m.Conns++
	h.mu.Unlock()
}

// onControl dispatches one decoded control frame.
func (h *Hub) onControl(s *Session, f Frame) {
	switch f.T {
	case TReg:
		h.handleReg(s, f)
	case TCode:
		h.handleCode(s, f)
	case TClaim:
		h.handleClaim(s, f)
	case TOpen:
		h.handleOpen(s, f)
	case TAccept:
		h.handleAccept(s, f)
	case TClose:
		h.handleClose(s, f)
	default:
		// hello is consumed by the endpoint handshake; anything else is a
		// protocol violation.
		h.fail(s, ErrProtocol, "unexpected frame "+f.T)
	}
}

// handleReg registers a daemon. The sign is shape-checked only — R1 trusts
// first-registration (the secret never leaves the daemon); R2 upgrades to
// PAKE (doc §6.4).
func (h *Hub) handleReg(s *Session, f Frame) {
	if s.Role() != RoleDaemon {
		h.fail(s, ErrProtocol, "reg requires daemon role")
		return
	}
	if !validToken(f.DevID, maxDevIDLen) || !validToken(f.Sign, maxSignLen) {
		h.fail(s, ErrProtocol, "reg requires devId and sign")
		return
	}
	h.mu.Lock()
	if old, ok := h.daemons[f.DevID]; ok && old != s {
		// same daemon re-connected elsewhere — replace the stale session
		h.mu.Unlock()
		old.Close()
		h.mu.Lock()
	}
	s.SetRole(RoleDaemon, f.DevID)
	h.daemons[f.DevID] = s
	h.mu.Unlock()
	h.log.Info("daemon registered", "devId", f.DevID, "remote", s.RemoteIP())
}

// handleCode records a pairing code advertised by an online daemon.
func (h *Hub) handleCode(s *Session, f Frame) {
	if s.Role() != RoleDaemon || s.DevID() == "" {
		h.fail(s, ErrProtocol, "code frame before reg")
		return
	}
	if !validToken(f.Code, maxCodeLen) {
		h.fail(s, ErrProtocol, "bad pairing code")
		return
	}
	ttl := time.Duration(f.TTL) * time.Second
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	if ttl > maxCodeTTL {
		ttl = maxCodeTTL
	}
	now := h.now()
	h.mu.Lock()
	h.codes[f.Code] = codeEntry{devID: s.DevID(), expiresAt: now.Add(ttl)}
	h.purgeCodesLocked(now)
	h.m.CodeReg++
	h.mu.Unlock()
	h.log.Info("pairing code registered", "devId", s.DevID(), "ttl", ttl)
	// ack so the daemon knows the code is live (and tests can sequence)
	_ = s.WriteControl(context.Background(), Frame{T: TCode, Code: f.Code, TTL: int(ttl.Seconds())})
}

// handleClaim consumes a pairing code and binds the client to the code's
// daemon. Rate limited per client IP (5/min, then 10min blacklist).
func (h *Hub) handleClaim(s *Session, f Frame) {
	if s.Role() != RoleClient {
		h.fail(s, ErrProtocol, "claim requires client role")
		return
	}
	ip := s.RemoteIP()
	now := h.now()
	h.mu.Lock()
	lim := h.limiters[ip]
	if lim == nil {
		lim = newIPLimiter()
		h.limiters[ip] = lim
	}
	if !lim.allow(now) {
		h.mu.Unlock()
		h.fail(s, ErrRateLimited, "too many claims from this address")
		return
	}
	h.m.Claims++

	entry, ok := h.codes[f.Code]
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(h.codes, f.Code)
		}
		h.mu.Unlock()
		h.fail(s, ErrInvalidCode, "unknown, expired or already used code")
		return
	}
	// one-time: consume on hit
	delete(h.codes, f.Code)
	h.claimed[s] = entry.devID
	h.m.ClaimHit++
	h.mu.Unlock()
	h.log.Info("code claimed", "devId", entry.devID, "remote", ip)
	_ = s.WriteControl(context.Background(), Frame{T: TClaim, DevID: entry.devID})
}

// handleOpen opens a tunnel stream. The client names the daemon either
// explicitly (devId, daily connections) or implicitly (its last claim).
// The relay allocates the streamId and relays the open to the daemon.
func (h *Hub) handleOpen(s *Session, f Frame) {
	if s.Role() != RoleClient {
		h.fail(s, ErrProtocol, "open requires client role")
		return
	}
	h.mu.Lock()
	devID := f.DevID
	if devID == "" {
		devID = h.claimed[s]
	}
	if devID == "" {
		h.mu.Unlock()
		h.fail(s, ErrProtocol, "open without devId (claim first)")
		return
	}
	if len(h.bySess[s]) >= maxStreamsPerClient {
		h.mu.Unlock()
		h.fail(s, ErrTooManyStream, "client stream limit reached")
		return
	}
	daemon := h.daemons[devID]
	if daemon == nil {
		h.mu.Unlock()
		h.fail(s, ErrDaemonOffline, "daemon is not connected")
		return
	}
	if len(h.bySess[daemon]) >= maxStreamsPerDaemon {
		h.mu.Unlock()
		h.fail(s, ErrTooManyStream, "daemon stream limit reached")
		return
	}
	h.nextID++
	id := h.nextID
	h.streams[id] = &pipe{id: id, client: s, daemon: daemon, devID: devID}
	h.bySess[s] = append(h.bySess[s], id)
	h.bySess[daemon] = append(h.bySess[daemon], id)
	h.mu.Unlock()

	// relay the open to the daemon and ack the id to the client
	if err := daemon.WriteControl(context.Background(), Frame{T: TOpen, StreamID: id}); err != nil {
		h.dropStream(id, "daemon write failed")
		return
	}
	if err := s.WriteControl(context.Background(), Frame{T: TOpen, StreamID: id}); err != nil {
		h.dropStream(id, "client write failed")
		return
	}
	h.log.Debug("stream opened", "streamId", id, "devId", devID)
}

// handleAccept relays the daemon's accept (tunnel stream established).
func (h *Hub) handleAccept(s *Session, f Frame) {
	if s.Role() != RoleDaemon {
		h.fail(s, ErrProtocol, "accept requires daemon role")
		return
	}
	h.mu.Lock()
	p, ok := h.streams[f.StreamID]
	h.mu.Unlock()
	if !ok || p.daemon != s {
		h.fail(s, ErrProtocol, "accept for unknown stream")
		return
	}
	_ = p.client.WriteControl(context.Background(), Frame{T: TAccept, StreamID: f.StreamID})
}

// handleClose relays a close to the peer and drops the pipe.
func (h *Hub) handleClose(s *Session, f Frame) {
	h.mu.Lock()
	p, ok := h.streams[f.StreamID]
	h.mu.Unlock()
	if !ok {
		return // already gone — idempotent
	}
	if p.client != s && p.daemon != s {
		h.fail(s, ErrProtocol, "close for foreign stream")
		return
	}
	peer := p.daemon
	if s == p.daemon {
		peer = p.client
	}
	h.dropStream(f.StreamID, f.Why)
	_ = peer.WriteControl(context.Background(), Frame{T: TClose, StreamID: f.StreamID, Why: f.Why})
}

// onData routes a binary data frame to the other end of the stream.
func (h *Hub) onData(s *Session, streamID uint32, payload []byte) {
	h.mu.Lock()
	p, ok := h.streams[streamID]
	h.mu.Unlock()
	if !ok {
		// stale or unknown stream — tell the sender instead of dropping the
		// whole connection (late packets after close are normal)
		_ = s.WriteControl(context.Background(), Frame{T: TClose, StreamID: streamID, Why: "unknown stream"})
		return
	}
	peer := p.daemon
	if s == p.daemon {
		peer = p.client
	}
	if err := peer.WriteData(context.Background(), streamID, payload); err != nil {
		h.dropStream(streamID, "peer write failed")
		return
	}
	h.mu.Lock()
	h.m.Relayed += int64(len(payload))
	h.mu.Unlock()
}

// dropStream removes a pipe (if present) and notifies both ends.
func (h *Hub) dropStream(id uint32, why string) {
	h.mu.Lock()
	p, ok := h.streams[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.streams, id)
	h.removeSessionStreamLocked(p.client, id)
	h.removeSessionStreamLocked(p.daemon, id)
	h.mu.Unlock()
	_ = p.client.WriteControl(context.Background(), Frame{T: TClose, StreamID: id, Why: why})
	_ = p.daemon.WriteControl(context.Background(), Frame{T: TClose, StreamID: id, Why: why})
}

// onSessionClose cleans up everything owned by a dying session.
func (h *Hub) onSessionClose(s *Session) {
	h.mu.Lock()
	h.m.Conns--
	delete(h.claimed, s)
	if s.Role() == RoleDaemon && s.DevID() != "" {
		if cur, ok := h.daemons[s.DevID()]; !ok || cur == s {
			delete(h.daemons, s.DevID())
		}
		// drop codes owned by this daemon
		for c, e := range h.codes {
			if e.devID == s.DevID() {
				delete(h.codes, c)
			}
		}
	}
	ids := append([]uint32{}, h.bySess[s]...)
	delete(h.bySess, s)
	pipes := make([]*pipe, 0, len(ids))
	for _, id := range ids {
		if p, ok := h.streams[id]; ok {
			delete(h.streams, id)
			pipes = append(pipes, p)
		}
	}
	h.mu.Unlock()

	for _, p := range pipes {
		peer := p.daemon
		if s == p.daemon {
			peer = p.client
		}
		_ = peer.WriteControl(context.Background(), Frame{T: TClose, StreamID: p.id, Why: "peer disconnected"})
	}
	h.log.Info("session closed", "role", s.Role(), "devId", s.DevID(), "streams", len(pipes))
}

// Close shuts the hub down, notifying every live session (graceful stop).
func (h *Hub) Close() {
	h.mu.Lock()
	sessions := make([]*Session, 0, len(h.daemons)+len(h.bySess))
	seen := map[*Session]bool{}
	for _, s := range h.daemons {
		if !seen[s] {
			seen[s] = true
			sessions = append(sessions, s)
		}
	}
	for s := range h.bySess {
		if !seen[s] {
			seen[s] = true
			sessions = append(sessions, s)
		}
	}
	h.mu.Unlock()
	for _, s := range sessions {
		s.Close() // fires onSessionClose → full cleanup
	}
}

// fail reports a protocol/policy error and disconnects the offender.
func (h *Hub) fail(s *Session, code, why string) {
	h.log.Warn("relay error frame", "code", code, "why", why, "remote", s.RemoteIP())
	_ = s.WriteControl(context.Background(), Frame{T: TError, Code: code, Why: why})
	s.Close()
}

// purgeCodesLocked drops expired codes (lazy cleanup). Caller holds h.mu.
func (h *Hub) purgeCodesLocked(now time.Time) {
	for c, e := range h.codes {
		if now.After(e.expiresAt) {
			delete(h.codes, c)
		}
	}
}

func (h *Hub) removeSessionStreamLocked(s *Session, id uint32) {
	ids := h.bySess[s]
	for i, v := range ids {
		if v == id {
			h.bySess[s] = append(ids[:i:i], ids[i+1:]...)
			return
		}
	}
}

// validToken is a shape check for short opaque tokens (non-empty, printable,
// bounded).
func validToken(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7E {
			return false
		}
	}
	return true
}
