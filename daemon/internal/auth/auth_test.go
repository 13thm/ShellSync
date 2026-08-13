package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shellsync/daemon/internal/repository"
)

func tempDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.db")
}

func TestVerifyLocalToken(t *testing.T) {
	v := NewVerifier("lock-secret", nil)

	uid, ok := v.Verify(context.Background(), "lock-secret")
	if !ok || uid != repository.DefaultUserID {
		t.Fatalf("local token: uid=%q ok=%v", uid, ok)
	}
	if _, ok := v.Verify(context.Background(), "wrong"); ok {
		t.Fatal("wrong token should fail")
	}
	if _, ok := v.Verify(context.Background(), ""); ok {
		t.Fatal("empty token should fail")
	}
}

func TestVerifyDeviceToken(t *testing.T) {
	dbPath := tempDB(t)
	db, err := repository.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	repository.Migrate(ctx, db)
	repository.SeedDefaults(ctx, db)

	devRepo := repository.NewDeviceRepo(db)
	dev, err := devRepo.Create(ctx, repository.DeviceCreate{
		UserID: repository.DefaultUserID, Name: "Phone", Platform: "ios", SessionToken: "dev-tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	v := NewVerifier("lock-secret", devRepo)
	uid, ok := v.Verify(context.Background(), "dev-tok")
	if !ok || uid != dev.UserID {
		t.Fatalf("device token: uid=%q ok=%v", uid, ok)
	}
}
