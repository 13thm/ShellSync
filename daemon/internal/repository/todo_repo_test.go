package repository

import (
	"context"
	"testing"
)

func TestTodoCRUD(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	tasks := NewTaskRepo(db)
	terms := NewTerminalRepo(db)
	repo := NewTodoRepo(db)

	task, _ := tasks.Create(ctx, TaskCreate{UserID: DefaultUserID, Name: "T"})
	term, _ := terms.Create(ctx, TerminalCreate{UserID: DefaultUserID, TaskID: task.ID, ShellType: "bash"})

	todo, err := repo.Create(ctx, TodoCreate{
		UserID: DefaultUserID, TaskID: task.ID, TerminalID: term.ID,
		Title: "Review PR", Content: "see #123", Priority: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if todo.Status != "pending" {
		t.Fatalf("status = %q", todo.Status)
	}
	if todo.TaskID != task.ID || todo.TerminalID != term.ID {
		t.Fatalf("links wrong: %+v", todo)
	}

	// toggle done
	upd, err := repo.Update(ctx, todo.ID, TodoPatch{Status: strPtr("done")})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != "done" {
		t.Fatalf("status = %q", upd.Status)
	}

	// list by task
	byTask, err := repo.ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byTask) != 1 {
		t.Fatalf("byTask len = %d", len(byTask))
	}

	// filter by status
	done, _ := repo.List(ctx, DefaultUserID, TodoFilter{Status: "done"})
	if len(done) != 1 {
		t.Fatalf("done len = %d", len(done))
	}

	// unbind task ("" -> NULL)
	upd2, err := repo.Update(ctx, todo.ID, TodoPatch{TaskID: strPtr("")})
	if err != nil {
		t.Fatal(err)
	}
	if upd2.TaskID != "" {
		t.Fatalf("task not unbound: %q", upd2.TaskID)
	}

	// incremental
	since, _ := repo.ListSince(ctx, DefaultUserID, 0)
	if len(since) != 1 {
		t.Fatalf("since len = %d", len(since))
	}

	// delete
	if err := repo.Delete(ctx, todo.ID); err != nil {
		t.Fatal(err)
	}
}
