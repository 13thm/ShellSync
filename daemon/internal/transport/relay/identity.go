package relay

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/google/uuid"
)

// Identity is the daemon's cloud identity. devSecret never leaves the
// machine (only its HMAC signatures do); devId is the routing handle the
// phone learns from the pairing QR code.
type Identity struct {
	DevID     string
	DevSecret string
}

// LoadOrCreateIdentity reads (or first generates) the identity from the
// settings KV store. Keys: relay.devid / relay.secret (see config.SettingsKey*).
func LoadOrCreateIdentity(ctx context.Context, store SettingsStore) (Identity, error) {
	devID, ok, err := store.Get(ctx, settingsKeyDevID)
	if err != nil {
		return Identity{}, fmt.Errorf("read relay.devid: %w", err)
	}
	if !ok || devID == "" {
		devID = uuid.NewString()
		devID = devID[0:8] + devID[9:13] + devID[14:18] + devID[19:23] + devID[24:] // strip dashes
		if err := store.Set(ctx, settingsKeyDevID, devID); err != nil {
			return Identity{}, fmt.Errorf("write relay.devid: %w", err)
		}
	}

	secret, ok, err := store.Get(ctx, settingsKeySecret)
	if err != nil {
		return Identity{}, fmt.Errorf("read relay.secret: %w", err)
	}
	if !ok || len(secret) != 64 { // 32 bytes hex
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return Identity{}, fmt.Errorf("gen devSecret: %w", err)
		}
		secret = fmt.Sprintf("%x", buf)
		if err := store.Set(ctx, settingsKeySecret, secret); err != nil {
			return Identity{}, fmt.Errorf("write relay.secret: %w", err)
		}
	}
	return Identity{DevID: devID, DevSecret: secret}, nil
}

// SettingsStore is the subset of the settings repository the relay needs.
type SettingsStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
}

// settings KV keys (mirror of config.SettingsKey*; local to avoid an import
// cycle risk if config ever grows).
const (
	settingsKeyDevID  = "relay.devid"
	settingsKeySecret = "relay.secret"
)
