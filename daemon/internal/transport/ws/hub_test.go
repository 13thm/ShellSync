package ws

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/shellsync/daemon/internal/auth"
	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/repository"
	"github.com/shellsync/daemon/internal/service"
	"github.com/shellsync/daemon/internal/terminal"
)

func setupHub(t *testing.T) (*Hub, *service.Services, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := repository.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repository.Migrate(ctx, db)
	repository.SeedDefaults(ctx, db)
	t.Cleanup(func() { db.Close() })

	termRepo := repository.NewTerminalRepo(db)
	logRepo := repository.NewLogRepo(db)
	todoRepo := repository.NewTodoRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	deviceRepo := repository.NewDeviceRepo(db)
	settingsRepo := repository.NewSettingsRepo(db)

	logMgr := logstore.NewManager(logRepo, logstore.Config{
		FlushWindow: 20 * time.Millisecond,
		LogsPath:    filepath.Join(t.TempDir(), "logs"),
	})
	t.Cleanup(logMgr.CloseAll)

	termMgr := terminal.NewManager(termRepo, logMgr)
	t.Cleanup(termMgr.CloseAll)

	bus := eventbus.New()
	svc := service.New(service.Deps{
		UserID: repository.DefaultUserID, TaskRepo: taskRepo, TerminalRepo: termRepo,
		TodoRepo: todoRepo, LogRepo: logRepo, DeviceRepo: deviceRepo, SettingsRepo: settingsRepo,
		TermMgr: termMgr, LogMgr: logMgr, Bus: bus,
	})
	verifier := auth.NewVerifier("test-token", deviceRepo)
	hub := NewHub(verifier, svc.Terminals, logMgr, bus)
	return hub, svc, "test-token"
}

// dialWS connects a client to the hub over an httptest server.
func dialWS(t *testing.T, hub *Hub, token string) *websocket.Conn {
	t.Helper()
	ts := httptest.NewServer(hub)
	t.Cleanup(ts.Close)
	url := "ws" + ts.URL[4:] + "?token=" + token
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.SetReadLimit(32 << 20) // history pages can be large
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func readMsg(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func writeMsg(t *testing.T, c *websocket.Conn, m map[string]any) {
	t.Helper()
	b, _ := json.Marshal(m)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// WS connect with a valid token + app-level ping -> pong (M2-5).
func TestWSPingPong(t *testing.T) {
	hub, _, tok := setupHub(t)
	c := dialWS(t, hub, tok)

	writeMsg(t, c, map[string]any{"type": "ping", "id": "1"})
	m := readMsg(t, c)
	if m["type"] != "pong" {
		t.Fatalf("type = %v, want pong", m["type"])
	}
}

// A bad token is rejected with 401 before the upgrade (M2-5/M2-3).
func TestWSRejectsBadToken(t *testing.T) {
	hub, _, _ := setupHub(t)
	ts := httptest.NewServer(hub)
	defer ts.Close()
	url := "ws" + ts.URL[4:] + "?token=wrong"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("expected dial to fail with bad token")
	}
}

// A domain event published to the bus reaches the WS client (M2-6).
func TestWSEventBroadcast(t *testing.T) {
	hub, svc, tok := setupHub(t)
	c := dialWS(t, hub, tok)

	// allow the connection to register with the hub
	time.Sleep(150 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := svc.Tasks.Create(ctx, repository.TaskCreate{Name: "hi"}); err != nil {
		t.Fatal(err)
	}

	// read until we see the broadcast task.created event
	seen := false
	for i := 0; i < 20; i++ {
		if m := readMsg(t, c); m["type"] == "task.created" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatal("did not receive task.created via WS")
	}
}
