package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/internal/pgtest"
)

// pgtest connects with the workspace id "2026-WS", so the registry row has to
// carry that id for the join to resolve.
func TestActiveSemesterRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)

	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}

	got, err := pg.GetActiveSemester(ctx)
	if err != nil {
		t.Fatalf("GetActiveSemester: %v", err)
	}
	if got == nil {
		t.Fatal("active semester is nil")
	}
	if got.Database != "2026-WS" {
		t.Errorf("Database = %q, want %q", got.Database, "2026-WS")
	}
	// The logical semester comes from the registry, not from a second stored copy.
	if got.Semester != "2026 WS" {
		t.Errorf("Semester = %q, want %q", got.Semester, "2026 WS")
	}
}

func TestActiveSemesterMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetActiveSemester(t.Context())
	if err != nil {
		t.Fatalf("GetActiveSemester: %v", err)
	}
	if got != nil {
		t.Errorf("GetActiveSemester = %v, want nil", got)
	}
}

// TestActiveSemesterRenameIsPickedUp is the payoff of not storing the label
// twice: renaming the logical semester of a workspace is visible immediately
// instead of leaving a stale copy behind.
func TestActiveSemesterRenameIsPickedUp(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 SS', 2)`)
	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}
	exec(t, pg, `update semester set semester = '2026 WS' where id = '2026-WS'`)

	got, err := pg.GetActiveSemester(ctx)
	if err != nil {
		t.Fatalf("GetActiveSemester: %v", err)
	}
	if got.Semester != "2026 WS" {
		t.Errorf("Semester = %q, want %q -- the label is stored twice again", got.Semester, "2026 WS")
	}
}

// TestActiveSemesterUnknownWorkspaceIsRejected pins the difference to Mongo: the
// document there could name a database that had since been dropped, and the next
// start would try to resume into it.
func TestActiveSemesterUnknownWorkspaceIsRejected(t *testing.T) {
	pg := pgtest.NewDB(t)

	if err := pg.SaveActiveSemester(t.Context()); err == nil {
		t.Error("SaveActiveSemester accepted a workspace that is not in the registry")
	}
}

func TestActiveSemesterIsASingleton(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('2026-WS', '2026 WS', 2)`)
	exec(t, pg, `insert into semester (id, semester, schema_version)
	             values ('Test26SS-v2', '2026 SS', 2)`)

	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester: %v", err)
	}
	if err := pg.SaveActiveSemester(ctx); err != nil {
		t.Fatalf("SaveActiveSemester (second): %v", err)
	}

	if n := count(t, pg, "select count(*) from active_semester"); n != 1 {
		t.Errorf("active_semester rows = %d, want 1", n)
	}
}
