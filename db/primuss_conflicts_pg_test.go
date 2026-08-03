package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func seedConflict(t *testing.T, pg *db.PG, program string, ancode, other, students int) {
	t.Helper()
	exec(t, pg, `insert into primuss_conflict (semester_id, program, ancode, other_ancode, num_students)
	             values ('2026-WS', $1, $2, $3, $4)`, program, ancode, other, students)
}

func TestPrimussConflictsForAncode(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedPrimussExam(t, pg, "IF-B", 200, "Lineare Algebra")
	seedPrimussExam(t, pg, "IF-B", 300, "Programmieren")
	// Deliberately out of ancode order, to prove the query sorts.
	seedConflict(t, pg, "IF-B", 100, 300, 4)
	seedConflict(t, pg, "IF-B", 100, 200, 7)

	got, err := pg.GetPrimussConflictsForAncode(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussConflictsForAncode: %v", err)
	}
	if got.AnCode != 100 {
		t.Errorf("AnCode = %d, want 100", got.AnCode)
	}
	// Module and examer come from the exam catalogue, not from a second copy.
	if got.Module != "Analysis" {
		t.Errorf("Module = %q, want %q", got.Module, "Analysis")
	}
	if got.MainExamer != "Braun O." {
		t.Errorf("MainExamer = %q, want %q", got.MainExamer, "Braun O.")
	}
	if len(got.Conflicts) != 2 {
		t.Fatalf("len(Conflicts) = %d, want 2", len(got.Conflicts))
	}
	if got.Conflicts[0].AnCode != 200 || got.Conflicts[0].NumberOfStuds != 7 {
		t.Errorf("Conflicts[0] = %+v, want {200 7}", got.Conflicts[0])
	}
	if got.Conflicts[1].AnCode != 300 || got.Conflicts[1].NumberOfStuds != 4 {
		t.Errorf("Conflicts[1] = %+v, want {300 4}", got.Conflicts[1])
	}
}

// An exam without conflicts still answers, with an empty list. Under MongoDB it
// had a conflicts document carrying only AnCo/Titel/Prüfer, and callers read the
// module and examer off the result either way.
func TestPrimussConflictsForAncodeWithoutAny(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")

	got, err := pg.GetPrimussConflictsForAncode(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussConflictsForAncode: %v", err)
	}
	if got == nil {
		t.Fatal("conflicts are nil for an exam that exists")
	}
	if got.Module != "Analysis" {
		t.Errorf("Module = %q, want %q", got.Module, "Analysis")
	}
	if got.Conflicts == nil {
		t.Error("Conflicts is nil, want an empty slice")
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("len(Conflicts) = %d, want 0", len(got.Conflicts))
	}
}

// The diagonal is kept on purpose: 37 rows in the live conflicts_DE carry their
// own ancode, and the value is the registration count rather than a conflict.
// conflictToModelConflicts passed it through, so this does too -- dropping it is
// a behaviour change that needs a consumer audit, not a storage migration.
func TestPrimussConflictsKeepTheDiagonal(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedConflict(t, pg, "IF-B", 100, 100, 42)

	got, err := pg.GetPrimussConflictsForAncode(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetPrimussConflictsForAncode: %v", err)
	}
	if len(got.Conflicts) != 1 {
		t.Fatalf("len(Conflicts) = %d, want 1", len(got.Conflicts))
	}
	if got.Conflicts[0].AnCode != 100 || got.Conflicts[0].NumberOfStuds != 42 {
		t.Errorf("diagonal = %+v, want {100 42}", got.Conflicts[0])
	}
}

// A counterpart ancode must be an exam of the same program. Checked against
// 7357 conflict entries across four programs of 2026-SS: every one of them is.
// Under MongoDB a counterpart was a field NAME, so nothing could enforce it, and
// a key the decoder could not parse silently became ancode 0.
func TestPrimussConflictCounterpartMustExist(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")

	if _, err := pg.PoolForTest().Exec(t.Context(),
		`insert into primuss_conflict (semester_id, program, ancode, other_ancode, num_students)
		 values ('2026-WS', 'IF-B', 100, 999, 3)`); err == nil {
		t.Error("a conflict against an exam that does not exist was accepted")
	}
	// And ancode 0, the value the old decoder produced for an unparsable key.
	if _, err := pg.PoolForTest().Exec(t.Context(),
		`insert into primuss_conflict (semester_id, program, ancode, other_ancode, num_students)
		 values ('2026-WS', 'IF-B', 100, 0, 3)`); err == nil {
		t.Error("a conflict against ancode 0 was accepted -- that was the old decoder's silent failure mode")
	}
}

func TestPrimussConflictsPerAncode(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B", "WD-B")
	for _, ancode := range []int{100, 200, 300} {
		seedPrimussExam(t, pg, "IF-B", ancode, "Modul")
	}
	seedPrimussExam(t, pg, "WD-B", 100, "Anderes Programm")
	seedConflict(t, pg, "IF-B", 100, 200, 7)
	seedConflict(t, pg, "IF-B", 100, 300, 4)
	seedConflict(t, pg, "IF-B", 200, 100, 7)

	perAncode, err := pg.GetPrimussConflictsPerAncode(ctx, "IF-B")
	if err != nil {
		t.Fatalf("GetPrimussConflictsPerAncode: %v", err)
	}
	// Every exam of the program is a key, including the one without conflicts.
	if len(perAncode) != 3 {
		t.Fatalf("len = %d, want 3 (one per exam of the program)", len(perAncode))
	}
	if n := len(perAncode[100].Conflicts); n != 2 {
		t.Errorf("exam 100 has %d conflicts, want 2", n)
	}
	if n := len(perAncode[200].Conflicts); n != 1 {
		t.Errorf("exam 200 has %d conflicts, want 1", n)
	}
	if perAncode[300] == nil {
		t.Fatal("exam 300 is missing from the map -- an exam without conflicts still has an entry")
	}
	if n := len(perAncode[300].Conflicts); n != 0 {
		t.Errorf("exam 300 has %d conflicts, want 0", n)
	}
	// The other program's exam must not leak in.
	if _, ok := perAncode[100]; ok && perAncode[100].Module != "Modul" {
		t.Errorf("module = %q -- the other program's exam leaked in", perAncode[100].Module)
	}
}

func TestPrimussConflictsOnlyPlanned(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	for _, ancode := range []int{100, 200, 300} {
		seedPrimussExam(t, pg, "IF-B", ancode, "Modul")
	}
	seedConflict(t, pg, "IF-B", 100, 200, 7)
	seedConflict(t, pg, "IF-B", 100, 300, 4)

	got, err := pg.GetPrimussConflictsForAncodeOnlyPlanned(ctx, "IF-B", 100,
		[]*model.ZPAExam{{AnCode: 300}})
	if err != nil {
		t.Fatalf("GetPrimussConflictsForAncodeOnlyPlanned: %v", err)
	}
	if len(got.Conflicts) != 1 {
		t.Fatalf("len(Conflicts) = %d, want 1", len(got.Conflicts))
	}
	if got.Conflicts[0].AnCode != 300 {
		t.Errorf("Conflicts[0].AnCode = %d, want 300", got.Conflicts[0].AnCode)
	}
}

// ChangeAncodeInConflicts reads back what the cascade already did. The rename
// itself belongs to ChangeAncode now, and FixPrimussAncode calls that first.
func TestChangeAncodeInConflictsReadsBackTheCascade(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedPrimussExam(t, pg, "IF-B", 200, "Lineare Algebra")
	seedConflict(t, pg, "IF-B", 100, 200, 7)

	if _, err := pg.ChangeAncode(ctx, "IF-B", 100, 111); err != nil {
		t.Fatalf("ChangeAncode: %v", err)
	}

	got, err := pg.ChangeAncodeInConflicts(ctx, "IF-B", 100, 111)
	if err != nil {
		t.Fatalf("ChangeAncodeInConflicts: %v", err)
	}
	if got == nil {
		t.Fatal("conflicts are nil after the rename")
	}
	if got.AnCode != 111 {
		t.Errorf("AnCode = %d, want 111", got.AnCode)
	}
	if len(got.Conflicts) != 1 || got.Conflicts[0].AnCode != 200 {
		t.Errorf("Conflicts = %+v, want one entry against 200", got.Conflicts)
	}
}

func TestChangeAncodeInConflictsWithoutAnExam(t *testing.T) {
	pg := pgtest.NewDB(t)

	seedPrimussFixtures(t, pg, "IF-B")

	got, err := pg.ChangeAncodeInConflicts(t.Context(), "IF-B", 100, 111)
	if err != nil {
		t.Fatalf("ChangeAncodeInConflicts: %v", err)
	}
	if got != nil {
		t.Errorf("ChangeAncodeInConflicts = %v, want nil when there is no such exam", got)
	}
}
