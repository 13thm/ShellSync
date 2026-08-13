// Package auth authenticates HTTP/WS requests.
//
// Two token kinds are accepted:
//   - the local token (from the daemon lock file) used by the Desktop client
//   - a session token issued to a paired Mobile device
package auth

import (
	"context"

	"github.com/shellsync/daemon/internal/repository"
)

// Verifier checks a bearer token and returns the authenticated user id.
type Verifier struct {
	lockToken  string
	deviceRepo *repository.DeviceRepo
}

// NewVerifier creates a Verifier. lockToken is the daemon's local token.
func NewVerifier(lockToken string, deviceRepo *repository.DeviceRepo) *Verifier {
	return &Verifier{lockToken: lockToken, deviceRepo: deviceRepo}
}

// Verify returns the user id for a valid token. The local token maps to the
// default local user; a device session token maps to that device's owner.
//
// Note: session tokens are stored verbatim for O(1) lookup (MVP, local DB).
func (v *Verifier) Verify(ctx context.Context, token string) (userID string, ok bool) {
	if token == "" {
		return "", false
	}
	if token == v.lockToken {
		return repository.DefaultUserID, true
	}
	if v.deviceRepo != nil {
		dev, err := v.deviceRepo.GetByToken(ctx, token)
		if err == nil && dev.ID != "" {
			return dev.UserID, true
		}
	}
	return "", false
}
