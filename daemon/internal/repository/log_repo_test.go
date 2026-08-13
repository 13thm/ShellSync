package repository

import (
	"context"
	"testing"
)

func TestLogAppendReadRangeTailMaxSeq(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	terms := NewTerminalRepo(db)
	repo := NewLogRepo(db)

	term, _ := terms.Create(ctx, TerminalCreate{UserID: DefaultUserID, ShellType: "bash"})

	// append seq 1..3
	for i, d := range []string{"a", "b", "c"} {
		if err := repo.AppendChunk(ctx, term.ID, int64(i+1), "stdout", d, nowMs()); err != nil {
			t.Fatalf("append %d: %v", i+1, err)
		}
	}

	// MaxSeq
	max, _ := repo.MaxSeq(ctx, term.ID)
	if max != 3 {
		t.Fatalf("maxSeq = %d, want 3", max)
	}

	// ReadRange with paging
	out, hasMore, err := repo.ReadRange(ctx, term.ID, 1, 2)
	if err != nil {
		t.Fatalf("readRange: %v", err)
	}
	if len(out) != 2 || out[0].Seq != 1 || out[1].Seq != 2 {
		t.Fatalf("page1 = %+v", out)
	}
	if !hasMore {
		t.Fatal("expected hasMore")
	}

	out, hasMore, _ = repo.ReadRange(ctx, term.ID, 1, 10)
	if len(out) != 3 || hasMore {
		t.Fatalf("all = %+v hasMore=%v", out, hasMore)
	}

	// range starting mid-way
	out, _, _ = repo.ReadRange(ctx, term.ID, 2, 10)
	if len(out) != 2 || out[0].Seq != 2 {
		t.Fatalf("from2 = %+v", out)
	}

	// Tail (last 2, ascending)
	tail, err := repo.Tail(ctx, term.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 2 || tail[0].Seq != 2 || tail[1].Seq != 3 {
		t.Fatalf("tail = %+v", tail)
	}

	// empty terminal -> maxSeq 0, tail empty
	term2, _ := terms.Create(ctx, TerminalCreate{UserID: DefaultUserID, ShellType: "zsh"})
	if m, _ := repo.MaxSeq(ctx, term2.ID); m != 0 {
		t.Fatalf("empty maxSeq = %d", m)
	}
	if t2, _ := repo.Tail(ctx, term2.ID, 10); len(t2) != 0 {
		t.Fatalf("empty tail len = %d", len(t2))
	}

	// DeleteByTerminal
	n, err := repo.DeleteByTerminal(ctx, term.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("deleted = %d, want 3", n)
	}
	if m, _ := repo.MaxSeq(ctx, term.ID); m != 0 {
		t.Fatalf("after delete maxSeq = %d", m)
	}
}

// Deleting a terminal must cascade-delete its logs.
func TestTerminalDeleteCascadesLogs(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()
	terms := NewTerminalRepo(db)
	logs := NewLogRepo(db)

	term, _ := terms.Create(ctx, TerminalCreate{UserID: DefaultUserID, ShellType: "bash"})
	logs.AppendChunk(ctx, term.ID, 1, "stdout", "x", nowMs())
	logs.AppendChunk(ctx, term.ID, 2, "stdout", "y", nowMs())

	if err := terms.Delete(ctx, term.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if m, _ := logs.MaxSeq(ctx, term.ID); m != 0 {
		t.Fatalf("logs not cascaded, maxSeq = %d", m)
	}
}
