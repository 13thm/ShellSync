package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Session wraps one accepted WebSocket connection. It serializes writes,
// runs the read loop, dispatches decoded frames to hub callbacks, and
// keeps the connection alive with periodic protocol-level pings.
type Session struct {
	conn     *websocket.Conn
	log      *slog.Logger
	role     string // filled from the hello frame
	devID    string // filled at reg (daemon role)
	remoteIP string // peer IP (from the HTTP request), for rate limiting

	writeMu sync.Mutex
	closed  atomic.Bool
	closeMu sync.Mutex
	onClose []func(*Session)

	onControl func(*Session, Frame)
	onData    func(*Session, uint32, []byte)
}

// readLimit bounds a single WS message (control or data chunk + 8B header).
const readLimit = int64(MaxChunk + 8 + 1024)

// pingInterval / pingTimeout drive dead-connection detection.
const (
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second
)

// NewSession wraps an accepted connection. remoteAddr comes from the HTTP
// request (the WS conn does not expose it). The session is not started
// until Run is called.
func NewSession(conn *websocket.Conn, remoteAddr string, log *slog.Logger) *Session {
	if log == nil {
		log = slog.Default()
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	// data frames are up to MaxChunk+8 bytes; the library default (32KiB)
	// would reject them
	conn.SetReadLimit(readLimit)
	return &Session{conn: conn, log: log, remoteIP: host}
}

// Role returns the role announced in the hello frame ("" until then).
func (s *Session) Role() string { return s.role }

// DevID returns the daemon devId set at reg (daemon sessions only).
func (s *Session) DevID() string { return s.devID }

// SetRole records the hello role. Called by the handshake code.
func (s *Session) SetRole(role, devID string) { s.role, s.devID = role, devID }

// RemoteIP strips the port from the peer address for rate limiting.
func (s *Session) RemoteIP() string { return s.remoteIP }

// Handlers installs the hub callbacks. Must be called before Run.
func (s *Session) Handlers(onControl func(*Session, Frame), onData func(*Session, uint32, []byte)) {
	s.onControl, s.onData = onControl, onData
}

// OnClose registers a callback fired exactly once when the session ends
// (read error, ping timeout, or explicit Close).
func (s *Session) OnClose(fn func(*Session)) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed.Load() {
		go fn(s) // already closed — fire async to avoid holding the lock
		return
	}
	s.onClose = append(s.onClose, fn)
}

// WriteControl sends a control (JSON text) frame.
func (s *Session) WriteControl(ctx context.Context, f Frame) error {
	b, err := EncodeControl(f)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, b)
}

// WriteData sends a binary data frame for the given stream.
func (s *Session) WriteData(ctx context.Context, streamID uint32, payload []byte) error {
	b, err := EncodeData(streamID, payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(ctx, websocket.MessageBinary, b)
}

// ErrClosed is returned by writes after the session was closed locally.
var ErrClosed = errors.New("relay: session closed")

// Close terminates the connection and fires on-close callbacks once.
func (s *Session) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	// Best-effort graceful WebSocket close, then drop the TCP connection.
	_ = s.conn.Close(websocket.StatusGoingAway, "relay closing")
	s.closeMu.Lock()
	cbs := append([]func(*Session){}, s.onClose...)
	s.onClose = nil
	s.closeMu.Unlock()
	for _, fn := range cbs {
		fn(s)
	}
}

// Run blocks reading frames until the connection dies or ctx is cancelled.
// It also maintains the ping keepalive. Returns the terminal error.
func (s *Session) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.readLoop(ctx) }()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			s.Close()
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
			s.writeMu.Lock()
			err := s.conn.Ping(pingCtx)
			s.writeMu.Unlock()
			pingCancel()
			if err != nil {
				s.Close()
				<-done // reap read loop
				return fmt.Errorf("ping: %w", err)
			}
		}
	}
}

// readLoop decodes frames and dispatches them. Malformed frames end the
// session (protocol violation).
func (s *Session) readLoop(ctx context.Context) error {
	for {
		msgType, reader, err := s.conn.Reader(ctx)
		if err != nil {
			return err
		}
		switch msgType {
		case websocket.MessageText:
			b, err := readAll(reader, maxControlSize)
			if err != nil {
				return fmt.Errorf("read control: %w", err)
			}
			f, err := DecodeControl(b)
			if err != nil {
				return err
			}
			if s.onControl != nil {
				s.onControl(s, f)
			}
		case websocket.MessageBinary:
			b, err := readAll(reader, readLimit)
			if err != nil {
				return fmt.Errorf("read data: %w", err)
			}
			streamID, payload, err := DecodeData(b)
			if err != nil {
				return err
			}
			if s.onData != nil {
				s.onData(s, streamID, payload)
			}
		}
	}
}

func readAll(r io.Reader, limit int64) ([]byte, error) {
	b := make([]byte, 0, 1024)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if int64(len(b)) > limit {
			return nil, fmt.Errorf("message exceeds %d bytes", limit)
		}
		if errors.Is(err, io.EOF) {
			return b, nil // end of message
		}
	}
}
