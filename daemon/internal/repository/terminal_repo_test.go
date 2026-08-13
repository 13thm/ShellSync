package repository

import (
	"context"
	"testing"
)

func TestTerminalCRUD(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	repo := NewTerminalRepo(db)

	term, err := repo.Create(ctx, TerminalCreate{
		UserID: DefaultUserID, ShellType: "powershell", Cols: 120, Rows: 30, Cwd: `E:\code`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// defaults
	if term.Name != "Terminal" {
		t.Fatalf("name = %q, want Terminal", term.Name)
	}
	if term.Status != "running" {
		t.Fatalf("status = %q", term.Status)
	}
	if term.Cols != 120 || term.Rows != 30 {
		t.Fatalf("size = %dx%d", term.Cols, term.Rows)
	}
	if term.Cwd != `E:\code` {
		t.Fatalf("cwd = %q", term.Cwd)
	}
	if term.ExitCode != nil || term.OsPID != nil {
		t.Fatal("running terminal should have nil exit/pid")
	}

	// update status to exited with code 0
	zero := 0
	if err := repo.UpdateStatus(ctx, term.ID, "exited", &zero); err != nil {
		t.Fatalf("updateStatus: %v", err)
	}
	got, _ := repo.Get(ctx, term.ID)
	if got.Status != "exited" || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("after exit: %+v", got)
	}

	// set last seq
	if err := repo.SetLastSeq(ctx, term.ID, 42); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, term.ID)
	if got.LastSeq != 42 {
		t.Fatalf("lastSeq = %d", got.LastSeq)
	}

	// rename
	upd, err := repo.Update(ctx, term.ID, TerminalPatch{Name: strPtr("Build")})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "Build" {
		t.Fatalf("name = %q", upd.Name)
	}

	// list + filter
	all, _ := repo.List(ctx, DefaultUserID, TerminalFilter{})
	if len(all) != 1 {
		t.Fatalf("len = %d", len(all))
	}
	running, _ := repo.List(ctx, DefaultUserID, TerminalFilter{Status: "running"})
	if len(running) != 0 {
		t.Fatalf("running len = %d, want 0", len(running))
	}

	// incremental
	since, _ := repo.ListSince(ctx, DefaultUserID, 0)
	if len(since) != 1 {
		t.Fatalf("since len = %d", len(since))
	}
}

func TestMarkRunningAsCrashed(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	repo := NewTerminalRepo(db)

	repo.Create(ctx, TerminalCreate{UserID: DefaultUserID, ShellType: "bash"})
	repo.Create(ctx, TerminalCreate{UserID: DefaultUserID, ShellType: "zsh"})
	// one already exited
	exited, _ := repo.Create(ctx, TerminalCreate{UserID: DefaultUserID, ShellType: "cmd", Status: "exited"})

	n, err := repo.MarkRunningAsCrashed(ctx)
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if n != 2 {
		t.Fatalf("marked = %d, want 2", n)
	}
	// idempotent: no more running
	n2, _ := repo.MarkRunningAsCrashed(ctx)
	if n2 != 0 {
		t.Fatalf("second mark = %d, want 0", n2)
	}
	// exited one untouched
	got, _ := repo.Get(ctx, exited.ID)
	if got.Status != "exited" {
		t.Fatalf("exited terminal changed to %q", got.Status)
	}
}
