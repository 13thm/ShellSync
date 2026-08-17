// Package relay implements the daemon's outbound cloud-relay client: it
// keeps one long-lived WebSocket connection to the relay-server, registers
// the daemon identity, reports pairing codes, and pipes multiplexed tunnel
// streams to the local HTTP port. No inbound ports are required (CGNAT-safe).
package relay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Frame/protocol constants mirrored from the relay-server wire protocol
// (server/relay/frame.go). Duplicated on purpose: daemon and relay-server
// are separate deployables — the protocol is the contract.
const (
	tHello  = "hello"
	tReg    = "reg"
	tCode   = "code"
	tClaim  = "claim"
	tOpen   = "open"
	tAccept = "accept"
	tClose  = "close"
	tError  = "error"

	roleDaemon = "daemon"
	protoVer   = 1

	maxChunk       = 32 * 1024
	maxControlRead = 16 * 1024
	heartbeatEvery = 30 * time.Second
	heartbeatWait  = 10 * time.Second
	dialTimeout    = 10 * time.Second
	retryMin       = 1 * time.Second
	retryMax       = 60 * time.Second
	retryJitter    = 0.2
)

// control mirrors the wire control frame.
type control struct {
	T        string `json:"t"`
	Role     string `json:"role,omitempty"`
	Ver      int    `json:"ver,omitempty"`
	DevID    string `json:"devId,omitempty"`
	Sign     string `json:"sign,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
	Ts       int64  `json:"ts,omitempty"`
	Code     string `json:"code,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	StreamID uint32 `json:"streamId,omitempty"`
	Why      string `json:"why,omitempty"`
}

func encodeData(streamID uint32, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	out[0], out[1], out[2], out[3] = byte(streamID>>24), byte(streamID>>16), byte(streamID>>8), byte(streamID)
	n := uint32(len(payload))
	out[4], out[5], out[6], out[7] = byte(n>>24), byte(n>>16), byte(n>>8), byte(n)
	copy(out[8:], payload)
	return out
}

func decodeData(b []byte) (uint32, []byte, error) {
	if len(b) < 8 {
		return 0, nil, fmt.Errorf("data frame too short (%d)", len(b))
	}
	id := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	n := uint32(b[4])<<24 | uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7])
	if int(n) != len(b)-8 {
		return 0, nil, fmt.Errorf("data frame length mismatch (hdr %d, got %d)", n, len(b)-8)
	}
	return id, b[8:], nil
}

// State of the relay client connection.
type State string

const (
	StateOffline  State = "offline"  // trying / backed off
	StateOnline   State = "online"   // registered with the relay
	StateDisabled State = "disabled" // cloud turned off by the user
)

// Options configures the client.
type Options struct {
	URL       string // relay ws url
	HCPort    int    // local HTTP port streams are piped to
	DevID     string // stable device id (uuid hex)
	DevSecret string // hex 32B HMAC key
	Log       *slog.Logger
}

// Client maintains the outbound relay connection with reconnection and
// stream forwarding. Create with New; run with Run (blocking).
type Client struct {
	opts Options
	log  *slog.Logger

	mu      sync.Mutex
	conn    *websocket.Conn
	streams map[uint32]*tstream
	state   State
	lastErr string

	onState func(State, string)
}

// tstream is one tunnel stream piped to 127.0.0.1:HCPort.
type tstream struct {
	id   uint32
	tcp  net.Conn
	done chan struct{}
	once sync.Once
}

// New creates a Client (not yet connected).
func New(opts Options) *Client {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Client{opts: opts, log: opts.Log, streams: map[uint32]*tstream{}, state: StateOffline}
}

// OnState registers a callback for state transitions (also fires for the
// initial transition when Run connects).
func (c *Client) OnState(fn func(State, string)) {
	c.mu.Lock()
	c.onState = fn
	c.mu.Unlock()
}

func (c *Client) setState(s State, lastErr string) {
	c.mu.Lock()
	c.state = s
	c.lastErr = lastErr
	fn := c.onState
	c.mu.Unlock()
	if fn != nil {
		fn(s, lastErr)
	}
}

// State reports the current connection state and last error.
func (c *Client) State() (State, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state, c.lastErr
}

// Online reports whether the daemon is registered with the relay.
func (c *Client) Online() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == StateOnline
}

// ReportCode pushes a fresh pairing code to the relay (no-op when offline —
// the code stays valid for LAN pairing).
func (c *Client) ReportCode(code string, ttl time.Duration) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f := control{T: tCode, Code: code, TTL: int(ttl.Seconds())}
	c.log.Info("reporting pairing code to relay", "code", code, "ttl", ttl)
	// wait for the relay ack so callers can sequence (QR display etc.)
	if err := writeControl(ctx, conn, f); err != nil {
		c.log.Warn("report code failed", "err", err)
	}
}

// Run connects and keeps the connection alive until ctx is done. Reconnects
// with exponential backoff + jitter; re-registers after every reconnect.
func (c *Client) Run(ctx context.Context) error {
	backoff := retryMin
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.setState(StateOffline, errString(err))
		c.log.Warn("relay connection lost, retrying", "err", err, "backoff", backoff)

		jitter := time.Duration(float64(backoff) * retryJitter * (2*frac() - 1))
		wait := backoff + jitter
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
		backoff *= 2
		if backoff > retryMax {
			backoff = retryMax
		}
	}
}

// runOnce performs one full connection cycle: dial → hello → reg → serve.
func (c *Client) runOnce(ctx context.Context) error {
	c.mu.Lock()
	c.closeStreamsLocked()
	c.mu.Unlock()

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	conn, _, err := websocket.Dial(dialCtx, c.opts.URL, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.opts.URL, err)
	}
	// relay data frames exceed the library's 32KiB default read limit
	conn.SetReadLimit(maxChunk + 8 + 1024)
	// ensure background pings/close don't outlive us
	go func() {
		<-ctx.Done()
		_ = conn.CloseNow()
	}()

	// hello + reg
	if err := writeControl(ctx, conn, control{T: tHello, Role: roleDaemon, Ver: protoVer}); err != nil {
		conn.CloseNow()
		return fmt.Errorf("hello: %w", err)
	}
	if err := expectHelloAck(ctx, conn); err != nil {
		conn.CloseNow()
		return err
	}
	sign, nonce, ts := Sign(c.opts.DevSecret, c.opts.DevID)
	if err := writeControl(ctx, conn, control{T: tReg, DevID: c.opts.DevID, Sign: sign, Nonce: nonce, Ts: ts}); err != nil {
		conn.CloseNow()
		return fmt.Errorf("reg: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.setState(StateOnline, "")
	c.log.Info("relay connected", "url", c.opts.URL, "devId", c.opts.DevID)

	err = c.serve(ctx, conn)

	c.mu.Lock()
	c.conn = nil
	c.closeStreamsLocked()
	c.mu.Unlock()
	return err
}

// serve is the read loop + heartbeat pump for one connection.
func (c *Client) serve(ctx context.Context, conn *websocket.Conn) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- c.readLoop(ctx, conn) }()

	ticker := time.NewTicker(heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case err := <-serveErr:
			return err
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, heartbeatWait)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				_ = conn.CloseNow()
				<-serveErr
				return fmt.Errorf("heartbeat: %w", err)
			}
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "daemon stopping")
			<-serveErr
			return ctx.Err()
		}
	}
}

// readLoop dispatches frames from the relay.
func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		msgType, reader, err := conn.Reader(ctx)
		if err != nil {
			return err
		}
		switch msgType {
		case websocket.MessageText:
			b, err := readAll(reader, maxControlRead)
			if err != nil {
				return err
			}
			if err := c.handleControl(ctx, conn, b); err != nil {
				return err
			}
		case websocket.MessageBinary:
			b, err := readAll(reader, maxChunk+8+1024)
			if err != nil {
				return err
			}
			id, payload, err := decodeData(b)
			if err != nil {
				return err
			}
			if err := c.handleData(ctx, conn, id, payload); err != nil {
				return err
			}
		}
	}
}

func (c *Client) handleControl(ctx context.Context, conn *websocket.Conn, raw []byte) error {
	var f control
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("bad control frame: %w", err)
	}
	switch f.T {
	case tOpen:
		return c.openStream(ctx, conn, f.StreamID)
	case tClose:
		c.closeStream(f.StreamID, f.Why)
		return nil
	case tError:
		c.log.Warn("relay reported error", "code", f.Code, "why", f.Why)
		return nil
	case tCode, tClaim, tHello, tReg, tAccept:
		// acks we do not act on (code ack, spurious claim, …)
		return nil
	default:
		return fmt.Errorf("unknown control frame %q", f.T)
	}
}

// openStream dials the local HTTP port and pipes it to the tunnel stream.
func (c *Client) openStream(ctx context.Context, conn *websocket.Conn, id uint32) error {
	if id == 0 {
		return errors.New("open with streamId 0")
	}
	tcp, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", c.opts.HCPort), 5*time.Second)
	if err != nil {
		_ = writeControl(ctx, conn, control{T: tClose, StreamID: id, Why: "local dial failed"})
		return fmt.Errorf("dial local http: %w", err)
	}
	st := &tstream{id: id, tcp: tcp, done: make(chan struct{})}

	c.mu.Lock()
	if c.streams == nil {
		c.streams = map[uint32]*tstream{}
	}
	c.streams[id] = st
	c.mu.Unlock()

	if err := writeControl(ctx, conn, control{T: tAccept, StreamID: id}); err != nil {
		c.closeStream(id, "accept write failed")
		return err
	}
	c.log.Debug("tunnel stream accepted", "streamId", id, "local", tcp.LocalAddr())

	// pump local → relay (relay → local is driven by handleData)
	go c.pumpLocal(ctx, conn, st)
	return nil
}

// pumpLocal forwards bytes from the local TCP connection to the relay.
func (c *Client) pumpLocal(ctx context.Context, conn *websocket.Conn, st *tstream) {
	buf := make([]byte, maxChunk)
	for {
		n, err := st.tcp.Read(buf)
		if n > 0 {
			if werr := writeData(ctx, conn, st.id, buf[:n]); werr != nil {
				c.closeStream(st.id, "relay write failed")
				return
			}
		}
		if err != nil {
			// EOF or local error → tell the relay the stream is done
			_ = writeControl(context.Background(), conn, control{T: tClose, StreamID: st.id, Why: "local eof"})
			c.closeStream(st.id, "local eof")
			return
		}
	}
}

// handleData writes relay → local for one stream.
func (c *Client) handleData(ctx context.Context, conn *websocket.Conn, id uint32, payload []byte) error {
	c.mu.Lock()
	st := c.streams[id]
	c.mu.Unlock()
	if st == nil {
		// late packet after close — acknowledge so the relay stops
		_ = writeControl(ctx, conn, control{T: tClose, StreamID: id, Why: "unknown stream"})
		return nil
	}
	if _, err := st.tcp.Write(payload); err != nil {
		c.closeStream(id, "local write failed")
	}
	return nil
}

func (c *Client) closeStream(id uint32, why string) {
	c.mu.Lock()
	st := c.streams[id]
	delete(c.streams, id)
	c.mu.Unlock()
	if st != nil {
		c.closeTStream(st)
		c.log.Debug("tunnel stream closed", "streamId", id, "why", why)
	}
}

func (c *Client) closeStreamsLocked() {
	for id, st := range c.streams {
		c.closeTStream(st)
		delete(c.streams, id)
	}
}

func (c *Client) closeTStream(st *tstream) {
	st.once.Do(func() {
		_ = st.tcp.Close()
		close(st.done)
	})
}

// ---- wire helpers ----

func writeControl(ctx context.Context, conn *websocket.Conn, f control) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, b)
}

func writeData(ctx context.Context, conn *websocket.Conn, id uint32, payload []byte) error {
	wctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageBinary, encodeData(id, payload))
}

func expectHelloAck(ctx context.Context, conn *websocket.Conn) error {
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	msgType, reader, err := conn.Reader(rctx)
	if err != nil {
		return fmt.Errorf("hello ack: %w", err)
	}
	if msgType != websocket.MessageText {
		return errors.New("relay did not answer hello")
	}
	b, err := readAll(reader, maxControlRead)
	if err != nil {
		return err
	}
	var f control
	if err := json.Unmarshal(b, &f); err != nil || f.T != tHello {
		return fmt.Errorf("bad hello ack: %s", b)
	}
	return nil
}

func readAll(r io.Reader, limit int64) ([]byte, error) {
	b := make([]byte, 0, 1024)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return b, nil
			}
			return nil, err
		}
		if int64(len(b)) > limit {
			return nil, fmt.Errorf("message exceeds %d bytes", limit)
		}
	}
}

// Sign builds the reg signature: HMAC-SHA256(devSecret, devId|nonce|unix).
// The relay only checks the shape in R1 (it never stores the secret).
func Sign(devSecret, devID string) (sign, nonce string, ts int64) {
	nonce = randHex(8)
	ts = time.Now().Unix()
	key, _ := hex.DecodeString(devSecret)
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s|%s|%d", devID, nonce, ts)
	return hex.EncodeToString(mac.Sum(nil)), nonce, ts
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:n*2]
	}
	return hex.EncodeToString(b)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func frac() float64 {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return float64(b[0]) / 255.0
}
