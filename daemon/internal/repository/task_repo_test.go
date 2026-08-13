package repository

import (
	"context"
	"testing"
	"time"
)

func TestTaskCRUD(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	repo := NewTaskRepo(db)

	// create
	task, err := repo.Create(ctx, TaskCreate{
		UserID: DefaultUserID, Name: "Write daemon", Description: "M1 milestone",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.ID == "" {
		t.Fatal("empty id")
	}
	if task.Status != "pending" {
		t.Fatalf("status = %q, want pending", task.Status)
	}
	if task.Description != "M1 milestone" {
		t.Fatalf("desc = %q", task.Description)
	}

	// get
	got, err := repo.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Write daemon" {
		t.Fatalf("name = %q", got.Name)
	}

	// list (1 task)
	list, err := repo.List(ctx, DefaultUserID, TaskFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}

	// ensure subsequent writes land in a strictly later millisecond
	// (Windows default timer granularity is ~15ms).
	time.Sleep(25 * time.Millisecond)

	// partial update
	upd, err := repo.Update(ctx, task.ID, TaskPatch{Status: strPtr("running"), Color: strPtr("red")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.Status != "running" || upd.Color != "red" {
		t.Fatalf("upd = %+v", upd)
	}
	if upd.UpdatedAt <= task.UpdatedAt {
		t.Fatal("UpdatedAt not bumped")
	}

	// clear description ("" -> NULL -> "")
	upd2, err := repo.Update(ctx, task.ID, TaskPatch{Description: strPtr("")})
	if err != nil {
		t.Fatalf("update2: %v", err)
	}
	if upd2.Description != "" {
		t.Fatalf("desc not cleared: %q", upd2.Description)
	}

	// incremental sync: things changed since original creation
	since, err := repo.ListSince(ctx, DefaultUserID, task.UpdatedAt)
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(since) != 1 {
		t.Fatalf("since len = %d, want 1", len(since))
	}

	// filter by status
	pending, err := repo.List(ctx, DefaultUserID, TaskFilter{Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending len = %d, want 0", len(pending))
	}

	// get missing
	if _, err := repo.Get(ctx, "nope"); err == nil {
		t.Fatal("expected error for missing task")
	}

	// delete
	if err := repo.Delete(ctx, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, task.ID); err == nil {
		t.Fatal("expected error after delete")
	}
}

// Deleting a task must unbind (not delete) its terminals (ON DELETE SET NULL).
func TestTaskDeleteUnbindsTerminals(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	tasks := NewTaskRepo(db)
	terms := NewTerminalRepo(db)

	task, err := tasks.Create(ctx, TaskCreate{UserID: DefaultUserID, Name: "T"})
	if err != nil {
		t.Fatal(err)
	}
	term, err := terms.Create(ctx, TerminalCreate{
		UserID: DefaultUserID, TaskID: task.ID, ShellType: "bash",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := tasks.Delete(ctx, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	got, err := terms.Get(ctx, term.ID)
	if err != nil {
		t.Fatalf("terminal gone after task delete: %v", err)
	}
	if got.TaskID != "" {
		t.Fatalf("task_id should be unbound, got %q", got.TaskID)
	}
}
