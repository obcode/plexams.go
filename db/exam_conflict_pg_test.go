package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// TestAPairIsThesameGivenEitherWayRound is the fix the CSV import needed.
// plexams normalises with conflictcalc.NormPair before it calls in, but the CSV
// import does not (plexams/csv_export.go:869,908). Under MongoDB a hand-edited
// row with the ancodes swapped created a second document that no lookup could
// reach and every reader counted twice.
func TestAPairIsTheSameGivenEitherWayRound(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100, 200)

	if err := pg.UpsertCanShareSlot(ctx, 100, 200); err != nil {
		t.Fatalf("UpsertCanShareSlot: %v", err)
	}
	if err := pg.UpsertCanShareSlot(ctx, 200, 100); err != nil {
		t.Fatalf("UpsertCanShareSlot reversed: %v", err)
	}

	pairs, err := pg.CanShareSlotPairs(ctx)
	if err != nil {
		t.Fatalf("CanShareSlotPairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1 -- the reversed pair is the same pair", len(pairs))
	}
	if pairs[0] != [2]int{100, 200} {
		t.Errorf("pair = %v, want [100 200] (smaller ancode first)", pairs[0])
	}

	// And it can be removed from either end.
	removed, err := pg.DeleteCanShareSlot(ctx, 200, 100)
	if err != nil {
		t.Fatalf("DeleteCanShareSlot: %v", err)
	}
	if !removed {
		t.Error("DeleteCanShareSlot reported nothing removed for the reversed pair")
	}
	removed, err = pg.DeleteCanShareSlot(ctx, 100, 200)
	if err != nil {
		t.Fatalf("DeleteCanShareSlot: %v", err)
	}
	if removed {
		t.Error("DeleteCanShareSlot reported a removal that did not happen")
	}
}

func TestStudentConflictDecisions(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100, 200)

	if err := pg.UpsertDecision(ctx, 200, 100, "00000001", string(model.ConflictDecisionAccept)); err != nil {
		t.Fatalf("UpsertDecision: %v", err)
	}
	// The same decision the other way round replaces it, it does not add one.
	if err := pg.UpsertDecision(ctx, 100, 200, "00000001", string(model.ConflictDecisionVeto)); err != nil {
		t.Fatalf("UpsertDecision: %v", err)
	}
	if err := pg.UpsertDecision(ctx, 100, 200, "00000002", string(model.ConflictDecisionAccept)); err != nil {
		t.Fatalf("UpsertDecision: %v", err)
	}

	decisions, err := pg.StudentConflictDecisions(ctx)
	if err != nil {
		t.Fatalf("StudentConflictDecisions: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("got %d decisions, want 2", len(decisions))
	}
	if decisions[0].Decision != model.ConflictDecisionVeto {
		t.Errorf("decision for 00000001 = %v, want VETO (the later write)", decisions[0].Decision)
	}
	if decisions[0].Ancode1 != 100 || decisions[0].Ancode2 != 200 {
		t.Errorf("ancodes = %d/%d, want 100/200", decisions[0].Ancode1, decisions[0].Ancode2)
	}

	removed, err := pg.DeleteDecision(ctx, 200, 100, "00000001")
	if err != nil {
		t.Fatalf("DeleteDecision: %v", err)
	}
	if !removed {
		t.Error("DeleteDecision reported nothing removed")
	}
}

// A decision may name a student nobody knows: registrations are Primuss source
// data replaced on every import, and a decision has to outlive a re-import. That
// is why mtknr has no foreign key.
func TestADecisionOutlivesTheStudentItNames(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100, 200)

	if err := pg.UpsertDecision(ctx, 100, 200, "99999999",
		string(model.ConflictDecisionAccept)); err != nil {
		t.Fatalf("a decision about an unknown student was rejected: %v", err)
	}
}

// A decision about an exam that does not exist is rejected -- the two foreign
// keys are what retire the check in plexams/validate_db.go:394.
func TestAPairMustNameRealExams(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)

	if err := pg.UpsertCanShareSlot(ctx, 100, 999); err == nil {
		t.Error("a can-share-slot pair naming an exam that does not exist was accepted")
	}
	if err := pg.UpsertDecision(ctx, 100, 999, "00000001",
		string(model.ConflictDecisionAccept)); err == nil {
		t.Error("a decision naming an exam that does not exist was accepted")
	}
}

func TestExternalExamsRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)
	// A joint program must name the faculty that plans it -- the check in
	// 00001 makes joint_faculty and category = 'joint' the same statement.
	exec(t, pg, `insert into study_program (shortname, name, category, joint_faculty)
	             values ('DE', 'Data Engineering', 'joint', 'FK10')`)

	exam := &model.ZPAExam{
		Semester: "2026 WS", AnCode: 900, Module: "Statistik", MainExamer: "Extern",
		ExamType: "schriftlich", Duration: 90, Faculty: "FK10",
		Groups: []string{"DE2"},
		PrimussAncodes: []model.ZPAPrimussAncodes{
			{Program: "DE", Ancode: 42},
		},
	}
	if err := pg.AddExternalExam(ctx, exam); err != nil {
		t.Fatalf("AddExternalExam: %v", err)
	}

	got, err := pg.ExternalExam(ctx, 900)
	if err != nil {
		t.Fatalf("ExternalExam: %v", err)
	}
	if got.Module != "Statistik" || got.Faculty != "FK10" {
		t.Errorf("exam = %#v, want the stored module and faculty", got)
	}
	if len(got.PrimussAncodes) != 1 || got.PrimussAncodes[0].Ancode != 42 {
		t.Errorf("PrimussAncodes = %v, want DE:42", got.PrimussAncodes)
	}
	// The semester label comes from the registry, not from a stored column.
	if got.Semester != "2026 WS" {
		t.Errorf("Semester = %q, want the logical label from the registry", got.Semester)
	}

	if err := pg.SetExternalExamFaculty(ctx, 900, "FK03"); err != nil {
		t.Fatalf("SetExternalExamFaculty: %v", err)
	}
	exams, err := pg.ExternalExams(ctx)
	if err != nil {
		t.Fatalf("ExternalExams: %v", err)
	}
	if len(exams) != 1 || exams[0].Faculty != "FK03" {
		t.Errorf("ExternalExams = %#v, want one exam in FK03", exams)
	}
	if len(exams[0].PrimussAncodes) != 1 {
		t.Errorf("PrimussAncodes = %v, want the link back", exams[0].PrimussAncodes)
	}

	if err := pg.DeleteExternalExam(ctx, 900); err != nil {
		t.Fatalf("DeleteExternalExam: %v", err)
	}
	if exams, err := pg.ExternalExams(ctx); err != nil {
		t.Fatalf("ExternalExams: %v", err)
	} else if len(exams) != 0 {
		t.Errorf("%d external exams left after the delete", len(exams))
	}
}

// DeleteExternalExam must not touch a ZPA exam of the same ancode -- and cannot,
// because the source is part of the predicate. Removing an external exam by hand
// is a deliberate act; removing a ZPA one is the import's job, and the import
// never deletes.
func TestDeleteExternalExamLeavesZPAExamsAlone(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)

	if err := pg.DeleteExternalExam(ctx, 100); err != nil {
		t.Fatalf("DeleteExternalExam: %v", err)
	}
	if n := count(t, pg, `select count(*) from exam where semester_id='2026-WS'`); n != 1 {
		t.Errorf("the ZPA exam is gone -- DeleteExternalExam is not filtering on source")
	}
}

// Removing an external exam takes its plan entry and rooms with it. That is the
// cascade being correct for a deliberate act, and exactly the one the ZPA import
// must never trigger.
func TestRemovingAnExternalExamTakesItsPlacementWithIt(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)

	if err := pg.AddExternalExam(ctx, &model.ZPAExam{
		AnCode: 900, Module: "Statistik", MainExamer: "Extern",
		ExamType: "schriftlich", Duration: 90,
	}); err != nil {
		t.Fatalf("AddExternalExam: %v", err)
	}
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 900, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}
	if err := pg.ReplacePlannedRooms(ctx, []*model.PlannedRoom{
		{Ancode: 900, RoomName: "R1.046", Duration: 90},
	}); err != nil {
		t.Fatalf("ReplacePlannedRooms: %v", err)
	}

	if err := pg.DeleteExternalExam(ctx, 900); err != nil {
		t.Fatalf("DeleteExternalExam: %v", err)
	}
	for _, table := range []string{"plan_entry", "planned_room"} {
		if n := count(t, pg, `select count(*) from `+table+` where semester_id='2026-WS'`); n != 0 {
			t.Errorf("%s still has %d rows after the exam was removed", table, n)
		}
	}
}

func TestRemovePlanEntry(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	start := berlin(t, "2027-01-20 08:30")
	if _, err := pg.AddExamToSlot(ctx, &model.PlanEntry{Ancode: 100, Starttime: &start}); err != nil {
		t.Fatalf("AddExamToSlot: %v", err)
	}

	if err := pg.RemovePlanEntry(ctx, 100); err != nil {
		t.Fatalf("RemovePlanEntry: %v", err)
	}
	entry, err := pg.PlanEntry(ctx, 100)
	if err != nil {
		t.Fatalf("PlanEntry: %v", err)
	}
	if entry != nil {
		t.Errorf("PlanEntry = %#v, want nil", entry)
	}

	// The exam itself stays -- only its placement was removed.
	if n := count(t, pg, `select count(*) from exam where semester_id='2026-WS'`); n != 1 {
		t.Error("RemovePlanEntry removed the exam, not just its plan entry")
	}
	// And removing what is not there is not an error.
	if err := pg.RemovePlanEntry(ctx, 100); err != nil {
		t.Errorf("RemovePlanEntry on nothing: %v", err)
	}
}
