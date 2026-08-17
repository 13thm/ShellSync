// Command relay-probe is a manual test client for the relay: it simulates a
// phone. Given a pairing code (or a devId) it opens a tunnel stream and
// issues one HTTP request through it, printing the raw response. Use it for
// the R1-10 P0 checklist (tunnel reachability without a phone).
//
// Examples:
//
//	relay-probe -url ws://127.0.0.1:8788/ws -code 482913 -get /health
//	relay-probe -url ws://127.0.0.1:8788/ws -dev a3f9c2 -get /api/tasks
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
)

func main() {
	url := flag.String("url", "ws://127.0.0.1:8788/ws", "relay ws url")
	code := flag.String("code", "", "pairing code to claim (or use -dev)")
	dev := flag.String("dev", "", "daemon devId (skip claim)")
	get := flag.String("get", "/health", "HTTP path to GET through the tunnel")
	host := flag.String("host", "daemon.local", "HTTP Host header value")
	timeout := flag.Duration("timeout", 15*time.Second, "overall timeout")
	flag.Parse()

	if *code == "" && *dev == "" {
		fmt.Fprintln(os.Stderr, "relay-probe: need -code or -dev")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := run(ctx, *url, *code, *dev, *get, *host); err != nil {
		slog.Error("probe failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, url, code, dev, path, host string) error {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()
	// tunneled payloads can be 32KiB frames
	conn.SetReadLimit(64 * 1024)

	// hello
	if err := writeText(ctx, conn, `{"t":"hello","role":"client","ver":1}`); err != nil {
		return err
	}
	f, err := readText(ctx, conn)
	if err != nil || !strings.HasPrefix(f, `{"t":"hell`) {
		return fmt.Errorf("hello ack missing: %v %s", err, f)
	}

	// claim (optional)
	if code != "" {
		if err := writeText(ctx, conn, fmt.Sprintf(`{"t":"claim","code":%q}`, code)); err != nil {
			return err
		}
		f, err := readText(ctx, conn)
		if err != nil {
			return err
		}
		if !strings.Contains(f, `"t":"claim"`) {
			return fmt.Errorf("claim rejected: %s", f)
		}
		if dev == "" {
			dev = extractJSONString(f, "devId")
		}
		fmt.Printf("claim ok → devId=%s\n", dev)
	}

	// open
	if err := writeText(ctx, conn, fmt.Sprintf(`{"t":"open","devId":%q}`, dev)); err != nil {
		return err
	}
	var streamID uint32
	for {
		f, err := readText(ctx, conn)
		if err != nil {
			return err
		}
		switch {
		case strings.Contains(f, `"t":"accept"`):
			streamID = extractStreamID(f)
		case strings.Contains(f, `"t":"open"`):
			continue
		case strings.Contains(f, `"t":"error"`):
			return fmt.Errorf("open rejected: %s", f)
		default:
			return fmt.Errorf("unexpected frame: %s", f)
		}
		if streamID != 0 {
			break
		}
	}
	fmt.Printf("stream open → streamId=%d\n", streamID)

	// HTTP request through the tunnel
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, host)
	if err := writeBinary(ctx, conn, streamID, []byte(req)); err != nil {
		return err
	}

	fmt.Println("--- response ---")
	for {
		msgType, r, err := conn.Reader(ctx)
		if err != nil {
			return err // connection closed — normal after Connection: close
		}
		switch msgType {
		case websocket.MessageBinary:
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			if len(b) < 8 {
				return fmt.Errorf("short data frame")
			}
			id := binary.BigEndian.Uint32(b[0:4])
			if id != streamID {
				continue
			}
			os.Stdout.Write(b[8:])
		case websocket.MessageText:
			b, _ := io.ReadAll(r)
			fmt.Printf("[control] %s\n", b)
			if strings.Contains(string(b), `"t":"close"`) {
				fmt.Println("--- stream closed by peer ---")
				return nil
			}
		}
	}
}

func writeText(ctx context.Context, conn *websocket.Conn, s string) error {
	return conn.Write(ctx, websocket.MessageText, []byte(s))
}

func writeBinary(ctx context.Context, conn *websocket.Conn, streamID uint32, payload []byte) error {
	b := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(b[0:4], streamID)
	binary.BigEndian.PutUint32(b[4:8], uint32(len(payload)))
	copy(b[8:], payload)
	return conn.Write(ctx, websocket.MessageBinary, b)
}

func readText(ctx context.Context, conn *websocket.Conn) (string, error) {
	msgType, r, err := conn.Reader(ctx)
	if err != nil {
		return "", err
	}
	if msgType != websocket.MessageText {
		return "", fmt.Errorf("expected text message, got %v", msgType)
	}
	b, err := io.ReadAll(r)
	return string(b), err
}

// extractJSONString pulls a top-level string field from small control frames
// (test tooling only — no nested objects expected).
func extractJSONString(s, field string) string {
	idx := strings.Index(s, `"`+field+`":"`)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(field)+4:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func extractStreamID(s string) uint32 {
	idx := strings.Index(s, `"streamId":`)
	if idx < 0 {
		return 0
	}
	rest := s[idx+len(`"streamId":`):]
	var v uint32
	for _, c := range rest {
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + uint32(c-'0')
	}
	return v
}
