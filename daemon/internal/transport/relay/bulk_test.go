package relay

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	serverrelay "github.com/shellsync/relay-server/relay"
)

// TestTunnelBulkAndConcurrency covers the P0 checklist items:
//   - 10MB continuous transfer through one stream (chunking/backpressure)
//   - 3 concurrent streams with interleaved traffic (streamId isolation)
func TestTunnelBulkAndConcurrency(t *testing.T) {
	const tenMB = 10 * 1024 * 1024
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			buf := make([]byte, 64*1024)
			for i := range buf {
				buf[i] = byte(i)
			}
			sent := 0
			w.Header().Set("Content-Length", fmt.Sprint(tenMB))
			for sent < tenMB {
				n := 64 * 1024
				if tenMB-sent < n {
					n = tenMB - sent
				}
				if _, err := w.Write(buf[:n]); err != nil {
					return
				}
				sent += n
			}
		default:
			fmt.Fprintf(w, "hello from %s", r.URL.Path)
		}
	}))
	defer httpSrv.Close()
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(httpSrv.URL, "http://"))
	var hcPort int
	fmt.Sscan(portStr, &hcPort)

	hub := serverrelay.NewHub(nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)
	relaySrv := httptest.NewServer(mux)
	defer relaySrv.Close()
	defer hub.Close()
	wsURL := "ws" + relaySrv.URL[len("http"):] + "/ws"

	const devID = "dev-bulk"
	c := New(Options{URL: wsURL, HCPort: hcPort, DevID: devID, DevSecret: "bb"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = c.Run(ctx) }()
	waitOnline(t, c)

	// --- ① 10MB single stream ---
	phone := dialStub(t, wsURL)
	stream := openStubStream(t, phone, devID)
	phone.data(stream, []byte("GET /big HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))

	got := 0
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		raw := phone.read(nil)
		id, payload, err := decodeData(raw)
		if err != nil {
			continue // control frame
		}
		if id != stream {
			t.Fatalf("crossed stream: want %d got %d", stream, id)
		}
		got += len(payload)
		if got >= tenMB {
			break
		}
	}
	if got < tenMB {
		t.Fatalf("expected ≥10MB through tunnel, got %d bytes", got)
	}

	// --- ② 3 concurrent streams, interleaved ---
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := dialStub(t, wsURL)
			defer p.conn.CloseNow()
			s := openStubStream(t, p, devID)
			path := fmt.Sprintf("/echo%d", i)
			p.data(s, []byte(fmt.Sprintf("GET %s HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n", path)))
			var buf []byte
			for !strings.Contains(string(buf), path) {
				raw := p.read(nil)
				id, payload, err := decodeData(raw)
				if err != nil {
					continue
				}
				if id != s {
					errs <- fmt.Errorf("stream %d: crossed frames from %d", s, id)
					return
				}
				buf = append(buf, payload...)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// openStubStream performs the client-side open handshake and returns the
// accepted stream id.
func openStubStream(t *testing.T, c *stubClient, devID string) uint32 {
	t.Helper()
	c.text(map[string]any{"t": "open", "devId": devID})
	for {
		var f map[string]any
		c.read(&f)
		switch f["t"] {
		case "open":
			continue
		case "accept":
			return uint32(f["streamId"].(float64))
		case "error":
			t.Fatalf("open rejected: %v", f)
		}
	}
}
