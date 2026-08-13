package repository

import (
	"context"
	"testing"
)

func TestSettingsCRUD(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	repo := NewSettingsRepo(db)

	// missing
	v, ok, err := repo.Get(ctx, "default_shell")
	if err != nil || ok {
		t.Fatalf("missing get: v=%q ok=%v err=%v", v, ok, err)
	}

	// set + get
	if err := repo.Set(ctx, "default_shell", "powershell"); err != nil {
		t.Fatal(err)
	}
	v, ok, _ = repo.Get(ctx, "default_shell")
	if !ok || v != "powershell" {
		t.Fatalf("get = %q ok=%v", v, ok)
	}

	// upsert
	repo.Set(ctx, "default_shell", "bash")
	v, _, _ = repo.Get(ctx, "default_shell")
	if v != "bash" {
		t.Fatalf("upsert = %q", v)
	}

	// GetAll
	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all["default_shell"] != "bash" {
		t.Fatalf("getAll = %+v", all)
	}

	// delete
	repo.Delete(ctx, "default_shell")
	_, ok, _ = repo.Get(ctx, "default_shell")
	if ok {
		t.Fatal("delete failed")
	}
}

func TestDevicePairRevoke(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	repo := NewDeviceRepo(db)

	dev, err := repo.Create(ctx, DeviceCreate{
		UserID: DefaultUserID, Name: "iPhone 15", Platform: "ios", SessionToken: "tok-secret",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if dev.Revoked {
		t.Fatal("new device revoked")
	}

	// lookup by token
	got, err := repo.GetByToken(ctx, "tok-secret")
	if err != nil {
		t.Fatalf("getByToken: %v", err)
	}
	if got.ID != dev.ID {
		t.Fatal("id mismatch")
	}

	// list by user
	list, _ := repo.ListByUser(ctx, DefaultUserID)
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}

	// touch last seen
	if err := repo.TouchLastSeen(ctx, dev.ID); err != nil {
		t.Fatal(err)
	}

	// revoke -> token no longer resolves
	if err := repo.Revoke(ctx, dev.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByToken(ctx, "tok-secret"); err == nil {
		t.Fatal("revoked token still resolves")
	}
}

func TestUserGetDefault(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	repo := NewUserRepo(db)

	u, err := repo.GetDefault(ctx)
	if err != nil {
		t.Fatalf("getDefault: %v", err)
	}
	if u.ID != DefaultUserID {
		t.Fatalf("id = %q", u.ID)
	}
	if u.Username != "local" {
		t.Fatalf("username = %q", u.Username)
	}
}
