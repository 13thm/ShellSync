package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shellsync/daemon/internal/repository"
)

func newPairService(t *testing.T) (*PairService, *fakeClock) {
	t.Helper()
	db, err := repository.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := repository.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := repository.SeedDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	return &PairService{
		deviceRepo: repository.NewDeviceRepo(db),
		userID:     repository.DefaultUserID,
		codes:      map[string]pairCode{},
		now:        clk.Now,
	}, clk
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

// seedCode inserts a code the way Init does, without randomness.
func seedCode(s *PairService, code string, ttl time.Duration) {
	s.codes[code] = pairCode{expiresAt: s.nowOrDefault().Add(ttl)}
}

func TestVerifyHappyPath(t *testing.T) {
	s, _ := newPairService(t)
	seedCode(s, "123456", pairTTL)

	res, err := s.Verify(context.Background(), "123456", "phone", "android")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.SessionToken == "" || res.Device.ID == "" {
		t.Fatalf("result incomplete: %+v", res)
	}

	// code is one-time
	if _, err := s.Verify(context.Background(), "123456", "phone2", "ios"); err != ErrInvalidPairing {
		t.Fatalf("second verify should fail, got %v", err)
	}
}

func TestVerifyExpiredCode(t *testing.T) {
	s, clk := newPairService(t)
	seedCode(s, "111222", time.Second)

	clk.t = clk.t.Add(2 * time.Second)
	if _, err := s.Verify(context.Background(), "111222", "p", "x"); err != ErrInvalidPairing {
		t.Fatalf("expired code should fail, got %v", err)
	}
}

// TestVerifyLockout: 3 consecutive failures lock for 5 minutes — even a
// correct code is rejected during the window, and works again afterwards.
func TestVerifyLockout(t *testing.T) {
	s, clk := newPairService(t)
	seedCode(s, "999888", lockDuration+pairTTL)

	for i := 0; i < 3; i++ {
		if _, err := s.Verify(context.Background(), "000000", "p", "x"); err != ErrInvalidPairing {
			t.Fatalf("wrong code %d should fail, got %v", i, err)
		}
	}

	// locked: even the correct code is refused
	if _, err := s.Verify(context.Background(), "999888", "p", "x"); err != ErrPairingLocked {
		t.Fatalf("locked verify should return ErrPairingLocked, got %v", err)
	}

	// after 5 minutes the lock lifts
	clk.t = clk.t.Add(lockDuration + time.Second)
	res, err := s.Verify(context.Background(), "999888", "p", "x")
	if err != nil {
		t.Fatalf("verify after lock: %v", err)
	}
	if res.SessionToken == "" {
		t.Fatal("no token after lock lifted")
	}
}

// TestVerifyLockoutResetsOnSuccess: a success in between keeps counting from
// zero.
func TestVerifyLockoutResetsOnSuccess(t *testing.T) {
	s, _ := newPairService(t)

	for round := 0; round < 3; round++ {
		seedCode(s, "555555", pairTTL)
		// two failures…
		for i := 0; i < 2; i++ {
			if _, err := s.Verify(context.Background(), "000000", "p", "x"); err != ErrInvalidPairing {
				t.Fatalf("round %d fail %d: %v", round, i, err)
			}
		}
		// …then a success resets the counter
		if _, err := s.Verify(context.Background(), "555555", "p", "x"); err != nil {
			t.Fatalf("round %d success: %v", round, err)
		}
	}
	// one more failure must NOT lock (counter was reset)
	seedCode(s, "777777", pairTTL)
	if _, err := s.Verify(context.Background(), "000000", "p", "x"); err != ErrInvalidPairing {
		t.Fatalf("expected invalid pairing, got %v", err)
	}
	if _, err := s.Verify(context.Background(), "777777", "p", "x"); err != nil {
		t.Fatalf("verify should still work, got %v", err)
	}
}

// --- QR payload (R1-6) ---

type fakeRelay struct {
	online    bool
	devID     string
	cloudHost string
}

func (f *fakeRelay) Online() bool                     { return f.online }
func (f *fakeRelay) ReportCode(string, time.Duration) {}
func (f *fakeRelay) CloudHostForQR(string) string {
	if !f.online {
		return ""
	}
	return f.cloudHost
}
func (f *fakeRelay) DevID() string { return f.devID }

func TestQRPayloadV2WhenCloudOnline(t *testing.T) {
	s, _ := newPairService(t)
	s.port = 8787
	s.relay = &fakeRelay{online: true, devID: "a3f9c2", cloudHost: "relay.example.com"}

	got := s.qrPayload("192.168.1.5", "482913")
	want := "shellsync://pair?v=2&code=482913&lan=192.168.1.5:8787&cloud=relay.example.com&dev=a3f9c2"
	if got != want {
		t.Fatalf("v2 payload mismatch:\n got  %s\n want %s", got, want)
	}
}

func TestQRPayloadFallsBackToV1(t *testing.T) {
	s, _ := newPairService(t)
	s.port = 8787

	// no relay at all
	got := s.qrPayload("192.168.1.5", "482913")
	want := "shellsync://pair?ip=192.168.1.5&port=8787&code=482913"
	if got != want {
		t.Fatalf("v1 payload mismatch (no relay):\n got  %s\n want %s", got, want)
	}

	// relay present but offline
	s.relay = &fakeRelay{online: false}
	if got := s.qrPayload("192.168.1.5", "482913"); got != want {
		t.Fatalf("v1 payload mismatch (offline relay): %s", got)
	}

	// relay online but cloud host unmappable (loopback w/o lan IP)
	s.relay = &fakeRelay{online: true, cloudHost: ""}
	if got := s.qrPayload("192.168.1.5", "482913"); got != want {
		t.Fatalf("v1 payload mismatch (no cloud host): %s", got)
	}
}
