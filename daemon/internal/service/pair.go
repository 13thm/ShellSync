package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/shellsync/daemon/internal/repository"
)

const pairTTL = 2 * time.Minute

// Pairing lockout policy: after maxVerifyFails consecutive wrong codes the
// verify endpoint refuses everything for lockDuration (brakes 6-digit code
// brute force; applies to LAN and cloud paths alike).
const (
	maxVerifyFails = 3
	lockDuration   = 5 * time.Minute
)

// ErrPairingLocked is returned while the lockout window is active.
var ErrPairingLocked = errors.New("pairing temporarily locked after repeated failures")

// RelayLink is the cloud-relay surface PairService needs (implemented by
// transport/relay.Manager). It breaks the dependency direction: service
// never imports transport.
type RelayLink interface {
	Online() bool
	ReportCode(code string, ttl time.Duration)
	CloudHostForQR(lanIP string) string
	DevID() string
}

type pairCode struct {
	expiresAt time.Time
}

// PairInitResult is returned by PairService.Init.
type PairInitResult struct {
	PairingCode string `json:"pairingCode"`
	QRPayload   string `json:"qrPayload"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// PairVerifyResult is returned by PairService.Verify.
type PairVerifyResult struct {
	SessionToken string            `json:"sessionToken"`
	Device       repository.Device `json:"device"`
}

// PairService handles device pairing (LAN + cloud relay).
type PairService struct {
	deviceRepo *repository.DeviceRepo
	userID     string

	mu    sync.Mutex
	codes map[string]pairCode
	port  int

	// verify failure lockout (in-memory; resets on restart)
	failCount   int
	lockedUntil time.Time

	// now is injectable for tests.
	now func() time.Time

	relay RelayLink
}

// SetPort records the HTTP port (used to build the QR payload). Call after the
// server binds.
func (s *PairService) SetPort(port int) { s.port = port }

// SetRelay wires the cloud relay link (optional; without it the service
// stays LAN-only and QR payloads fall back to v1).
func (s *PairService) SetRelay(r RelayLink) { s.relay = r }

// Init generates a one-time pairing code and reports it to the cloud relay
// when connected. The QR payload is v2 when the cloud path is live (lan +
// cloud + dev), else v1 (ip/port/code) — old apps scan both.
func (s *PairService) Init(_ context.Context) (PairInitResult, error) {
	code := randomDigits(6)
	now := time.Now()
	s.mu.Lock()
	s.codes[code] = pairCode{expiresAt: now.Add(pairTTL)}
	s.purge(now)
	s.mu.Unlock()

	// cloud report (best effort; LAN pairing still works when offline)
	if s.relay != nil && s.relay.Online() {
		s.relay.ReportCode(code, pairTTL)
	}

	ip := lanIP()
	expiresAt := now.Add(pairTTL)
	return PairInitResult{
		PairingCode: code,
		QRPayload:   s.qrPayload(ip, code),
		ExpiresAt:   expiresAt.UnixMilli(),
	}, nil
}

// qrPayload builds the QR string.
//
//	v2 (cloud reachable):
//	  shellsync://pair?v=2&code=<6>&lan=<ip:port>&cloud=<host[:port]>&dev=<devId>
//	v1 (fallback):
//	  shellsync://pair?ip=<ip>&port=<port>&code=<6>
func (s *PairService) qrPayload(ip, code string) string {
	if s.relay != nil && s.relay.Online() {
		if cloud := s.relay.CloudHostForQR(ip); cloud != "" {
			return fmt.Sprintf(
				"shellsync://pair?v=2&code=%s&lan=%s:%d&cloud=%s&dev=%s",
				code, ip, s.port, cloud, s.relay.DevID())
		}
	}
	return fmt.Sprintf("shellsync://pair?ip=%s&port=%d&code=%s", ip, s.port, code)
}

// Verify exchanges a pairing code for a device session token. Consecutive
// failures trigger a temporary lockout (see ErrPairingLocked).
func (s *PairService) Verify(ctx context.Context, code, name, platform string) (PairVerifyResult, error) {
	if err := s.checkCode(code); err != nil {
		return PairVerifyResult{}, err
	}

	token := randomToken(32)
	dev, err := s.deviceRepo.Create(ctx, repository.DeviceCreate{
		UserID:       s.userID,
		Name:         name,
		Platform:     platform,
		SessionToken: token,
	})
	if err != nil {
		return PairVerifyResult{}, err
	}
	return PairVerifyResult{SessionToken: token, Device: dev}, nil
}

// checkCode validates + consumes a code and maintains the failure lockout.
func (s *PairService) checkCode(code string) error {
	now := s.nowOrDefault()
	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Before(s.lockedUntil) {
		return ErrPairingLocked
	}
	entry, ok := s.codes[code]
	if ok && now.After(entry.expiresAt) {
		delete(s.codes, code)
		ok = false
	}
	if ok {
		delete(s.codes, code) // one-time
		s.failCount = 0
		return nil
	}
	s.failCount++
	if s.failCount >= maxVerifyFails {
		s.lockedUntil = now.Add(lockDuration)
		s.failCount = 0
	}
	return ErrInvalidPairing
}

func (s *PairService) nowOrDefault() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// purge removes expired codes. Caller must hold s.mu.
func (s *PairService) purge(now time.Time) {
	for c, e := range s.codes {
		if now.After(e.expiresAt) {
			delete(s.codes, c)
		}
	}
}

func randomDigits(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "000000"
	}
	out := make([]byte, n)
	for i, v := range b {
		out[i] = '0' + (v % 10)
	}
	return string(out)
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(b)
}

// lanIP returns the most likely LAN IPv4 address, or 127.0.0.1.
// Strategy: dial a remote address via UDP (no packets sent) to learn the
// primary outbound (default-route) interface IP — this naturally picks the
// real network adapter over virtual ones (VMware/WSL/link-local).
// Falls back to scanning interface addrs for a private IPv4.
func lanIP() string {
	if ip := outboundIP(); ip != "" {
		return ip
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		ip4 := ipnet.IP.To4()
		if ip4 == nil || ip4.IsLinkLocalUnicast() {
			continue
		}
		if ip4.IsPrivate() {
			return ip4.String()
		}
	}
	return "127.0.0.1"
}

// outboundIP discovers the IP of the interface facing the default route.
func outboundIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsLoopback() {
		return ""
	}
	if ip4 := addr.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() {
		return ip4.String()
	}
	return ""
}
