package relay

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"
)

// Manager owns the relay Client lifecycle: it starts/stops it on user
// toggle (settings key cloud.enabled), tracks state, and implements the
// interface the pairing service uses to publish codes (service.RelayLink).
type Manager struct {
	opts    Options
	baseCtx context.Context // daemon-lifetime context (NOT a request ctx)

	mu      sync.Mutex
	client  *Client
	cancel  context.CancelFunc
	enabled bool
	running bool

	state   State
	lastErr string
	log     *slog.Logger
}

// NewManager creates a manager. enabled is the initial (config) value.
func NewManager(opts Options, enabled bool) *Manager {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Manager{opts: opts, enabled: enabled, state: StateDisabled, log: opts.Log}
}

// Start launches the relay client if enabled (idempotent). The context must
// outlive the call (daemon lifetime); SetEnabled reuses it for restarts.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.baseCtx == nil {
		m.baseCtx = ctx
	}
	if m.running || !m.enabled {
		m.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(m.baseCtx)
	client := New(m.opts)
	m.client = client
	m.cancel = cancel
	m.running = true
	m.state = StateOffline
	m.mu.Unlock()

	client.OnState(func(s State, lastErr string) {
		m.mu.Lock()
		m.state, m.lastErr = s, lastErr
		m.mu.Unlock()
	})

	go func() {
		err := client.Run(runCtx)
		cancel()
		m.mu.Lock()
		if m.client == client {
			m.client = nil
			m.running = false
			if m.enabled {
				m.state = StateOffline
			} else {
				m.state = StateDisabled
			}
			m.lastErr = errString(err)
		}
		m.mu.Unlock()
	}()
}

// Stop tears the relay client down and marks it disabled.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.client = nil
	m.running = false
	m.state = StateDisabled
}

// SetEnabled reacts to the cloud.enabled settings toggle: true → (re)start,
// false → stop. Callers persist the value; the manager only acts. Safe to
// call from HTTP handlers (uses the daemon-lifetime context, not the
// request's).
func (m *Manager) SetEnabled(v bool) {
	if v {
		m.mu.Lock()
		m.enabled = true
		wasRunning := m.running
		ctx := m.baseCtx
		m.mu.Unlock()
		if !wasRunning {
			if ctx == nil {
				ctx = context.Background()
			}
			m.Start(ctx)
		}
		return
	}
	m.Stop()
}

// Status is the cloud snapshot exposed via GET /api/settings.
type Status struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url"`
	State     string `json:"state"` // online | offline | disabled
	LastError string `json:"lastError,omitempty"`
}

// StatusSnapshot returns the current cloud status.
func (m *Manager) StatusSnapshot() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.state
	if !m.enabled {
		st = StateDisabled
	}
	return Status{
		Enabled:   m.enabled,
		URL:       m.opts.URL,
		State:     string(st),
		LastError: m.lastErr,
	}
}

// --- service.RelayLink implementation (used by PairService) ---

// Online reports whether the relay connection is live.
func (m *Manager) Online() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled && m.state == StateOnline
}

// ReportCode pushes a pairing code to the relay.
func (m *Manager) ReportCode(code string, ttl time.Duration) {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client != nil {
		client.ReportCode(code, ttl)
	}
}

// DevID returns the daemon device id embedded in QR v2 payloads.
func (m *Manager) DevID() string { return m.opts.DevID }

// CloudHostForQR maps the configured relay URL to the host[:port] string the
// phone should connect to. Loopback hosts are replaced with the LAN IP so a
// dev relay on the daemon machine is reachable from the phone on the same
// network (staging). Default ports are stripped (wss→443 implies TLS host).
func (m *Manager) CloudHostForQR(lanIP string) string {
	return CloudHostFromURL(m.opts.URL, lanIP)
}

// CloudHostFromURL implements the QR "cloud" field mapping. Exported for tests.
func CloudHostFromURL(rawURL, lanIP string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	if host == "127.0.0.1" || host == "localhost" || host == "::1" {
		if lanIP == "" || lanIP == "127.0.0.1" {
			return "" // loopback relay is unreachable from the phone → v1 QR
		}
		host = lanIP
	}
	port := u.Port()
	if port == "" || (u.Scheme == "wss" && port == "443") || (u.Scheme == "ws" && port == "80") {
		return host
	}
	return net.JoinHostPort(host, port)
}
