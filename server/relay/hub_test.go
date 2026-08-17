package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testHub spins up an httptest server with the real /ws endpoint and a hub
// whose clock is controllable.
type testHub struct {
	hub *Hub
	srv *httptest.Server
	url string // ws://127.0.0.1:PORT/ws
	now time.Time
	mu  sync.Mutex
}

func newTestHub(t *testing.T) *testHub {
	t.Helper()
	th := &testHub{now: time.Now()}
	th.hub = NewHub(nil, func() time.Time { th.mu.Lock(); defer th.mu.Unlock(); return th.now })
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", th.hub.ServeWS)
	mux.HandleFunc("/metrics", th.hub.MetricsHandler())
	th.srv = httptest.NewServer(mux)
	th.url = "ws" + th.srv.URL[len("http"):] + "/ws"
	t.Cleanup(func() {
		th.hub.Close()
		th.srv.Close()
	})
	return th
}

func (th *testHub) advance(d time.Duration) {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.now = th.now.Add(d)
}

// testConn is a raw test client speaking the relay protocol.
type testConn struct {
	t    *testing.T
	conn *websocket.Conn
}

func dial(t *testing.T, url, role string) *testConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	c := &testConn{t: t, conn: conn}
	c.sendJSON(map[string]any{"t": "hello", "role": role, "ver": ProtocolVersion})
	// wait for the hello ack
	f := c.readControl()
	if f["t"] != THello {
		t.Fatalf("expected hello ack, got %v", f)
	}
	return c
}

func (c *testConn) sendJSON(v any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, _ := json.Marshal(v)
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		c.t.Fatalf("write %s: %v", b, err)
	}
}

func (c *testConn) sendData(streamID uint32, payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := EncodeData(streamID, payload)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.conn.Write(ctx, websocket.MessageBinary, b); err != nil {
		c.t.Fatalf("write data: %v", err)
	}
}

// readControl reads the next TEXT message and decodes it as a map.
func (c *testConn) readControl() map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgType, reader, err := c.conn.Reader(ctx)
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	if msgType != websocket.MessageText {
		c.t.Fatalf("expected text message, got %v", msgType)
	}
	b := make([]byte, 0, 256)
	buf := make([]byte, 1024)
	for {
		n, rerr := reader.Read(buf)
		b = append(b, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("bad json %s: %v", b, err)
	}
	return m
}

// readData reads the next BINARY message and decodes the streamId/payload.
func (c *testConn) readData() (uint32, []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgType, reader, err := c.conn.Reader(ctx)
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	if msgType != websocket.MessageBinary {
		c.t.Fatalf("expected binary message, got %v", msgType)
	}
	b := make([]byte, 0, 256)
	buf := make([]byte, 1024)
	for {
		n, rerr := reader.Read(buf)
		b = append(b, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	id, p, err := DecodeData(b)
	if err != nil {
		c.t.Fatalf("bad data frame: %v", err)
	}
	return id, p
}

// reg performs the daemon hello+reg handshake.
func (c *testConn) reg(devID string) {
	c.sendJSON(map[string]any{"t": "reg", "devId": devID, "sign": "00ff00ff", "nonce": "n1", "ts": 1})
}

// reportCode registers a pairing code and waits for the relay ack.
func (c *testConn) reportCode(code string, ttl int) {
	c.sendJSON(map[string]any{"t": "code", "code": code, "ttl": ttl})
	for {
		f := c.readControl()
		if f["t"] == TCode {
			return
		}
		if f["t"] == TError {
			c.t.Fatalf("code rejected: %v", f)
		}
	}
}

// openFromClient sends open and waits for the accept, returning streamId.
func (c *testConn) openFromClient(devID string) uint32 {
	c.sendJSON(map[string]any{"t": "open", "devId": devID})
	for {
		f := c.readControl()
		switch f["t"] {
		case TOpen:
			continue // id ack — keep waiting for accept
		case TAccept:
			return uint32(f["streamId"].(float64))
		case TError:
			c.t.Fatalf("open failed: %v", f)
		default:
			c.t.Fatalf("unexpected frame while opening: %v", f)
		}
	}
}

// acceptFromDaemon replies accept for a daemon-side open.
func (c *testConn) acceptStream(id uint32) {
	c.sendJSON(map[string]any{"t": "accept", "streamId": id})
}

// TestFullFlow walks the whole R1-3 pipeline:
// reg → code → claim → open → accept → bidirectional data → close.
func TestFullFlow(t *testing.T) {
	th := newTestHub(t)
	const devID = "dev-aaa111"

	daemon := dial(t, th.url, RoleDaemon)
	daemon.reg(devID)

	daemon.reportCode("482913", 120)

	client := dial(t, th.url, RoleClient)
	client.sendJSON(map[string]any{"t": "claim", "code": "482913"})
	ack := client.readControl()
	if ack["t"] != TClaim || ack["devId"] != devID {
		t.Fatalf("claim ack wrong: %v", ack)
	}

	// client opens a stream; the daemon sees the forwarded open
	client.sendJSON(map[string]any{"t": "open", "devId": devID})
	var streamID uint32
	for {
		f := daemon.readControl()
		if f["t"] == TOpen {
			streamID = uint32(f["streamId"].(float64))
			break
		}
		if f["t"] == TError {
			t.Fatalf("daemon got error: %v", f)
		}
	}
	daemon.acceptStream(streamID)
	// client receives the id ack, then the accept
	if f := client.readControl(); f["t"] != TOpen || uint32(f["streamId"].(float64)) != streamID {
		t.Fatalf("client open ack wrong: %v", f)
	}
	if f := client.readControl(); f["t"] != TAccept || uint32(f["streamId"].(float64)) != streamID {
		t.Fatalf("client accept wrong: %v", f)
	}

	// client → daemon
	client.sendData(streamID, []byte("ping from phone"))
	if id, p := daemon.readData(); id != streamID || string(p) != "ping from phone" {
		t.Fatalf("daemon received %d %q", id, p)
	}
	// daemon → client (echo)
	daemon.sendData(streamID, []byte("pong from daemon"))
	if id, p := client.readData(); id != streamID || string(p) != "pong from daemon" {
		t.Fatalf("client received %d %q", id, p)
	}

	// client closes → daemon gets close
	client.sendJSON(map[string]any{"t": "close", "streamId": streamID, "why": "done"})
	f := daemon.readControl()
	if f["t"] != TClose || uint32(f["streamId"].(float64)) != streamID {
		t.Fatalf("daemon close frame wrong: %v", f)
	}
}

// TestCodeExpiry: expired codes are rejected at claim time.
func TestCodeExpiry(t *testing.T) {
	th := newTestHub(t)
	daemon := dial(t, th.url, RoleDaemon)
	daemon.reg("dev-bbb222")
	daemon.reportCode("111222", 5)

	th.advance(6 * time.Second)

	client := dial(t, th.url, RoleClient)
	client.sendJSON(map[string]any{"t": "claim", "code": "111222"})
	if f := client.readControl(); f["t"] != TError || f["code"] != ErrInvalidCode {
		t.Fatalf("expected invalid_code error, got %v", f)
	}
}

// TestCodeOneShot: a code cannot be claimed twice.
func TestCodeOneShot(t *testing.T) {
	th := newTestHub(t)
	daemon := dial(t, th.url, RoleDaemon)
	daemon.reg("dev-ccc333")
	daemon.reportCode("333444", 120)

	c1 := dial(t, th.url, RoleClient)
	c1.sendJSON(map[string]any{"t": "claim", "code": "333444"})
	if f := c1.readControl(); f["t"] != TClaim {
		t.Fatalf("first claim failed: %v", f)
	}

	c2 := dial(t, th.url, RoleClient)
	c2.sendJSON(map[string]any{"t": "claim", "code": "333444"})
	if f := c2.readControl(); f["t"] != TError || f["code"] != ErrInvalidCode {
		t.Fatalf("second claim should fail, got %v", f)
	}
}

// TestClaimRateLimit: the 6th claim within a minute is rate limited.
func TestClaimRateLimit(t *testing.T) {
	th := newTestHub(t)
	for i := 0; i < 6; i++ {
		c := dial(t, th.url, RoleClient)
		c.sendJSON(map[string]any{"t": "claim", "code": "000000"})
		f := c.readControl()
		if i == 5 {
			if f["t"] != TError || f["code"] != ErrRateLimited {
				t.Fatalf("6th claim should be rate limited, got %v", f)
			}
		} else if f["code"] != ErrInvalidCode {
			t.Fatalf("claim %d should be invalid_code, got %v", i, f)
		}
		_ = c.conn.CloseNow()
	}
}

// TestOpenUnknownDaemon: opening a stream to an offline daemon errors.
func TestOpenUnknownDaemon(t *testing.T) {
	th := newTestHub(t)
	client := dial(t, th.url, RoleClient)
	client.sendJSON(map[string]any{"t": "open", "devId": "ghost"})
	if f := client.readControl(); f["t"] != TError || f["code"] != ErrDaemonOffline {
		t.Fatalf("expected daemon_offline, got %v", f)
	}
}

// TestDaemonDisconnectClosesStreams: killing the daemon side delivers close
// frames to the client.
func TestDaemonDisconnectClosesStreams(t *testing.T) {
	th := newTestHub(t)
	const devID = "dev-ddd444"

	daemon := dial(t, th.url, RoleDaemon)
	daemon.reg(devID)
	daemon.reportCode("555666", 120)

	client := dial(t, th.url, RoleClient)
	client.sendJSON(map[string]any{"t": "claim", "code": "555666"})
	client.readControl() // claim ack

	client.sendJSON(map[string]any{"t": "open", "devId": devID})
	var streamID uint32
	for {
		f := daemon.readControl()
		if f["t"] == TOpen {
			streamID = uint32(f["streamId"].(float64))
			break
		}
	}
	daemon.acceptStream(streamID)
	client.readControl() // open ack
	client.readControl() // accept

	// hard-kill the daemon connection
	_ = daemon.conn.Close(websocket.StatusNormalClosure, "bye")

	for {
		f := client.readControl()
		if f["t"] == TClose && uint32(f["streamId"].(float64)) == streamID {
			break
		}
		if f["t"] == TError {
			t.Fatalf("unexpected error: %v", f)
		}
	}
}

// TestBadFirstFrame: a non-hello first frame is rejected.
func TestBadFirstFrame(t *testing.T) {
	th := newTestHub(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, th.url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_ = conn.Write(ctx, websocket.MessageText, []byte(`{"t":"reg","devId":"x"}`))
	// read until we see the error frame (hello ack comes first is NOT the case
	// here — no hello was sent)
	msgType, reader, err := conn.Reader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.MessageText {
		t.Fatalf("expected text, got %v", msgType)
	}
	b := make([]byte, 256)
	n, _ := reader.Read(b)
	var f Frame
	if err := json.Unmarshal(b[:n], &f); err != nil {
		t.Fatal(err)
	}
	if f.T != TError || f.Code != ErrProtocol {
		t.Fatalf("expected protocol error, got %+v", f)
	}
}

// TestMetricsSmoke: metrics counters move.
func TestMetricsSmoke(t *testing.T) {
	th := newTestHub(t)
	d := dial(t, th.url, RoleDaemon)
	d.reg("dev-metrics")
	c := dial(t, th.url, RoleClient)
	_ = c

	// metrics endpoint via plain HTTP GET
	resp, err := th.srv.Client().Get(th.srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m Metrics
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Conns < 2 || m.Daemons < 1 {
		t.Fatalf("metrics look wrong: %+v", m)
	}
}
