package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/shellsync/daemon/internal/repository"
)

const pairTTL = 2 * time.Minute

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

// PairService handles device pairing (MVP: LAN, single user).
type PairService struct {
	deviceRepo *repository.DeviceRepo
	userID     string

	mu    sync.Mutex
	codes map[string]pairCode
	port  int
}

// SetPort records the HTTP port (used to build the QR payload). Call after the
// server binds.
func (s *PairService) SetPort(port int) { s.port = port }

// Init generates a one-time pairing code.
func (s *PairService) Init(_ context.Context) (PairInitResult, error) {
	code := randomDigits(6)
	s.mu.Lock()
	s.codes[code] = pairCode{expiresAt: time.Now().Add(pairTTL)}
	s.purge()
	s.mu.Unlock()

	ip := lanIP()
	return PairInitResult{
		PairingCode: code,
		QRPayload:   fmt.Sprintf("shellsync://pair?ip=%s&port=%d&code=%s", ip, s.port, code),
		ExpiresAt:   time.Now().Add(pairTTL).UnixMilli(),
	}, nil
}

// Verify exchanges a pairing code for a device session token.
func (s *PairService) Verify(ctx context.Context, code, name, platform string) (PairVerifyResult, error) {
	s.mu.Lock()
	entry, ok := s.codes[code]
	if ok && time.Now().After(entry.expiresAt) {
		delete(s.codes, code)
		ok = false
	}
	if ok {
		delete(s.codes, code)
	}
	s.mu.Unlock()
	if !ok {
		return PairVerifyResult{}, ErrInvalidPairing
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

// purge removes expired codes. Caller must hold s.mu.
func (s *PairService) purge() {
	now := time.Now()
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
