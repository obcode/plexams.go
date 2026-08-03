package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/internal/pgtest"
)

// TestChangeAncodeCascadesThroughConflicts is the payoff of the conflict redesign.
//
// Renumbering a Primuss exam used to be three writes: $set AnCo in exams_<P>,
// $set AnCo in count_<P>, and a $rename of one FIELD NAME across every document
// of conflicts_<P> -- because the counterpart ancodes were field names there. The
// data was inconsistent between the steps, and the rename was O(documents).
//
// Here the exam is the key, so one UPDATE moves the counter, the exam's own
// conflicts and every conflict that names it as a counterpart, atomically.
func TestChangeAncodeCascadesThroughConflicts(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	for _, ancode := range []int{100, 200, 300} {
		seedPrimussExam(t, pg, "IF-B", ancode, "Modul")
	}
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total)
	             values ('2026-WS', 'IF-B', 100, 42)`)
	exec(t, pg, `insert into primuss_conflict (semester_id, program, ancode, other_ancode, num_students)
	             values ('2026-WS', 'IF-B', 100, 200, 7),
	                    ('2026-WS', 'IF-B', 100, 100, 42),
	                    ('2026-WS', 'IF-B', 300, 100, 4)`)

	got, err := pg.ChangeAncode(ctx, "IF-B", 100, 111)
	if err != nil {
		t.Fatalf("ChangeAncode: %v", err)
	}
	if got.AnCode != 111 {
		t.Errorf("AnCode = %d, want 111", got.AnCode)
	}

	// The counter followed.
	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 111)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 42 {
		t.Errorf("count for the renumbered exam = %d, want 42", sum)
	}
	if n := count(t, pg, `select count(*) from primuss_count where ancode = 100`); n != 0 {
		t.Errorf("%d counter rows still carry the old ancode", n)
	}

	// The exam's own conflicts followed ...
	if n := count(t, pg, `select count(*) from primuss_conflict where ancode = 111`); n != 2 {
		t.Errorf("conflicts with the new ancode = %d, want 2", n)
	}
	// ... and so did the ones that merely name it as a counterpart, which is the
	// $rename that no longer has to be written.
	if n := count(t, pg,
		`select count(*) from primuss_conflict where ancode = 300 and other_ancode = 111`); n != 1 {
		t.Error("the counterpart reference was not renumbered")
	}
	// The diagonal row was renamed on both sides at once.
	if n := count(t, pg,
		`select count(*) from primuss_conflict where ancode = 111 and other_ancode = 111`); n != 1 {
		t.Error("the diagonal entry was not renumbered on both sides")
	}
	if n := count(t, pg,
		`select count(*) from primuss_conflict where ancode = 100 or other_ancode = 100`); n != 0 {
		t.Errorf("%d conflict rows still carry the old ancode", n)
	}
}

// Renumbering onto an ancode the program already uses is refused. Mongo would
// have produced two exams with the same ancode and left the conflicts pointing at
// whichever the decoder saw last.
func TestChangeAncodeOntoAnExistingOneIsRefused(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedPrimussExam(t, pg, "IF-B", 200, "Lineare Algebra")

	if _, err := pg.ChangeAncode(ctx, "IF-B", 100, 200); err == nil {
		t.Fatal("ChangeAncode created a second exam with an ancode already in use")
	}

	// Nothing moved.
	if n := count(t, pg, `select count(*) from primuss_exam where ancode = 100`); n != 1 {
		t.Error("the source exam is gone although the rename failed")
	}
}

// ChangeAncodeInStudentRegsCount is a no-op once the exam rename has cascaded,
// and must stay harmless -- ChangeAncodeInStudentRegs calls it unconditionally.
func TestChangeAncodeInCountAfterTheCascadeIsHarmless(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	exec(t, pg, `insert into primuss_count (semester_id, program, ancode, total)
	             values ('2026-WS', 'IF-B', 100, 42)`)

	if _, err := pg.ChangeAncode(ctx, "IF-B", 100, 111); err != nil {
		t.Fatalf("ChangeAncode: %v", err)
	}
	if err := pg.ChangeAncodeInStudentRegsCount(ctx, "IF-B", 100, 111); err != nil {
		t.Fatalf("ChangeAncodeInStudentRegsCount after the cascade: %v", err)
	}

	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 111)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 42 {
		t.Errorf("count = %d, want 42", sum)
	}
}

func TestPrimussCountMissingIsZero(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")

	sum, err := pg.GetStudentRegsCount(ctx, "IF-B", 100)
	if err != nil {
		t.Fatalf("GetStudentRegsCount: %v", err)
	}
	if sum != 0 {
		t.Errorf("count = %d, want 0 for an exam Primuss delivered no counter for", sum)
	}
}
