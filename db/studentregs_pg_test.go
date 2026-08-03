package db_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func seedPreparedStudent(t *testing.T, pg *db.PG, mtknr, name string) {
	t.Helper()
	blob, err := json.Marshal(&model.Student{Mtknr: mtknr, Name: name, Program: "IF-B"})
	if err != nil {
		t.Fatalf("marshal student: %v", err)
	}
	exec(t, pg, `insert into student_prepared (semester_id, mtknr, student, format_version)
	             values ('2026-WS', $1, $2, 1)`, mtknr, blob)
}

func preparedStudent(mtknr, name, program string) *model.Student {
	return &model.Student{
		Mtknr:      mtknr,
		Name:       name,
		Program:    program,
		Group:      "2",
		ZpaAncodes: []int{100, 200},
		RegsWithProgram: []*model.RegWithProgram{
			{Program: program, PrimussAncode: 100, ZpaAncode: 100},
		},
	}
}

func TestPreparedStudentsRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	want := preparedStudent("00012345", "Beispiel, Andrea", "IF-B")
	if err := pg.SaveStudentRegs(ctx, []interface{}{want}); err != nil {
		t.Fatalf("SaveStudentRegs: %v", err)
	}

	got, err := pg.StudentByMtknr(ctx, "00012345")
	if err != nil {
		t.Fatalf("StudentByMtknr: %v", err)
	}
	if got.Mtknr != want.Mtknr || got.Name != want.Name || got.Program != want.Program {
		t.Errorf("student = %+v, want %+v", got, want)
	}
	if len(got.ZpaAncodes) != 2 || got.ZpaAncodes[0] != 100 {
		t.Errorf("ZpaAncodes = %v, want [100 200]", got.ZpaAncodes)
	}
	if len(got.RegsWithProgram) != 1 || got.RegsWithProgram[0].PrimussAncode != 100 {
		t.Errorf("RegsWithProgram = %+v, want one entry for 100", got.RegsWithProgram)
	}

	n, err := pg.CountStudentRegsPlanned(ctx)
	if err != nil {
		t.Fatalf("CountStudentRegsPlanned: %v", err)
	}
	if n != 1 {
		t.Errorf("CountStudentRegsPlanned = %d, want 1", n)
	}
}

// SaveStudentRegs is a clear-and-refill: the students of the previous run must
// not survive it, or the cache would only ever grow.
func TestSaveStudentRegsReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	if err := pg.SaveStudentRegs(ctx, []interface{}{
		preparedStudent("00000001", "Erste, Person", "IF-B"),
		preparedStudent("00000002", "Zweite, Person", "IF-B"),
	}); err != nil {
		t.Fatalf("SaveStudentRegs: %v", err)
	}
	if err := pg.SaveStudentRegs(ctx, []interface{}{
		preparedStudent("00000003", "Dritte, Person", "IF-B"),
	}); err != nil {
		t.Fatalf("SaveStudentRegs (second): %v", err)
	}

	n, err := pg.CountStudentRegsPlanned(ctx)
	if err != nil {
		t.Fatalf("CountStudentRegsPlanned: %v", err)
	}
	if n != 1 {
		t.Errorf("CountStudentRegsPlanned = %d, want 1 -- the refill did not clear", n)
	}
}

// Sorting goes through the jsonb document rather than a duplicated name column,
// so there is nothing that can drift out of step with the blob.
func TestPreparedStudentsSortedByName(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	if err := pg.SaveStudentRegs(ctx, []interface{}{
		preparedStudent("00000003", "Zuletzt, Zacharias", "IF-B"),
		preparedStudent("00000001", "Anfang, Anna", "IF-B"),
		preparedStudent("00000002", "Mitte, Martin", "IF-B"),
	}); err != nil {
		t.Fatalf("SaveStudentRegs: %v", err)
	}

	students, err := pg.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		t.Fatalf("StudentRegsPerStudentPlanned: %v", err)
	}
	want := []string{"Anfang, Anna", "Mitte, Martin", "Zuletzt, Zacharias"}
	if len(students) != len(want) {
		t.Fatalf("len = %d, want %d", len(students), len(want))
	}
	for i, w := range want {
		if students[i].Name != w {
			t.Errorf("[%d].Name = %q, want %q", i, students[i].Name, w)
		}
	}
}

func TestPreparedStudentsEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	students, err := pg.StudentRegsPerStudentPlanned(t.Context())
	if err != nil {
		t.Fatalf("StudentRegsPerStudentPlanned: %v", err)
	}
	if students == nil {
		t.Fatal("StudentRegsPerStudentPlanned returned nil, want an empty slice")
	}
}

// StudentsByName searches the raw registrations and answers with prepared
// students -- a student who matched but was never prepared is skipped, as the
// Mongo per-student lookup did when it failed.
func TestStudentsByName(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B", "WD-B")
	seedPrimussExam(t, pg, "IF-B", 100, "Analysis")
	seedPrimussExam(t, pg, "WD-B", 100, "Anderes")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000001", "Meier, Anna")
	seedStudentReg(t, pg, "WD-B", "WD-B", 100, "00000002", "Meier, Berta")
	seedStudentReg(t, pg, "IF-B", "IF-B", 100, "00000003", "Schmidt, Clara")
	// Only the first two are prepared.
	if err := pg.SaveStudentRegs(ctx, []interface{}{
		preparedStudent("00000001", "Meier, Anna", "IF-B"),
		preparedStudent("00000002", "Meier, Berta", "WD-B"),
	}); err != nil {
		t.Fatalf("SaveStudentRegs: %v", err)
	}

	students, err := pg.StudentsByName(ctx, "^Meier")
	if err != nil {
		t.Fatalf("StudentsByName: %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("len = %d, want 2", len(students))
	}
	if students[0].Name != "Meier, Anna" || students[1].Name != "Meier, Berta" {
		t.Errorf("students = %q, %q", students[0].Name, students[1].Name)
	}

	// Matches a registration but no prepared student: skipped, not an error.
	students, err = pg.StudentsByName(ctx, "^Schmidt")
	if err != nil {
		t.Fatalf("StudentsByName: %v", err)
	}
	if len(students) != 0 {
		t.Errorf("len = %d, want 0", len(students))
	}

	students, err = pg.StudentsByName(ctx, "^Niemand")
	if err != nil {
		t.Fatalf("StudentsByName: %v", err)
	}
	if students == nil {
		t.Error("StudentsByName returned nil, want an empty slice")
	}
}

// A missing state row means nothing has been prepared yet, which is reported as
// not dirty rather than as an error -- the GUI banner would otherwise fire on a
// fresh semester.
func TestStudentRegsStateMissingIsNotDirty(t *testing.T) {
	pg := pgtest.NewDB(t)

	state, err := pg.GetStudentRegsState(t.Context())
	if err != nil {
		t.Fatalf("GetStudentRegsState: %v", err)
	}
	if state == nil {
		t.Fatal("state is nil")
	}
	if state.Dirty {
		t.Error("Dirty = true on a semester where nothing was ever prepared")
	}
}

// Reason and ChangedAt are what the GUI renders in the stale banner; the schema
// gained both columns because model.StudentRegsState carries them.
func TestStudentRegsStateRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")
	changedAt := time.Date(2026, 8, 4, 9, 30, 0, 0, time.Local)

	if err := pg.SetStudentRegsDirty(ctx, true, "addStudentReg", changedAt); err != nil {
		t.Fatalf("SetStudentRegsDirty: %v", err)
	}

	state, err := pg.GetStudentRegsState(ctx)
	if err != nil {
		t.Fatalf("GetStudentRegsState: %v", err)
	}
	if !state.Dirty {
		t.Error("Dirty = false right after it was set")
	}
	if strval(state.Reason) != "addStudentReg" {
		t.Errorf("Reason = %s, want %q", strval(state.Reason), "addStudentReg")
	}
	if state.ChangedAt == nil || !state.ChangedAt.Equal(changedAt) {
		t.Errorf("ChangedAt = %v, want %v", state.ChangedAt, changedAt)
	}

	// Regenerating clears it, and the empty reason stays unset rather than "".
	if err := pg.SetStudentRegsDirty(ctx, false, "", changedAt.Add(time.Hour)); err != nil {
		t.Fatalf("SetStudentRegsDirty (clear): %v", err)
	}
	state, err = pg.GetStudentRegsState(ctx)
	if err != nil {
		t.Fatalf("GetStudentRegsState: %v", err)
	}
	if state.Dirty {
		t.Error("Dirty = true after a regeneration")
	}
	if state.Reason != nil {
		t.Errorf("Reason = %q, want nil", *state.Reason)
	}
}

func TestRegsWithErrorsRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedPrimussFixtures(t, pg, "IF-B")

	want := []*model.RegWithError{
		{
			Registration: &model.ZPAStudentReg{AnCode: 100, Mtknr: "00000001", Program: "IF-B"},
			Error:        &model.ZPAStudentRegError{AnCode: "unbekannt"},
		},
		{
			Registration: &model.ZPAStudentReg{AnCode: 200, Mtknr: "00000002", Program: "IF-B"},
			Error:        &model.ZPAStudentRegError{Mtknr: "unbekannt"},
		},
	}
	if err := pg.SetRegsWithErrors(ctx, want); err != nil {
		t.Fatalf("SetRegsWithErrors: %v", err)
	}

	got, err := pg.GetRegsWithErrors(ctx)
	if err != nil {
		t.Fatalf("GetRegsWithErrors: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Registration.AnCode != 100 || got[0].Registration.Mtknr != "00000001" {
		t.Errorf("got[0].Registration = %+v", got[0].Registration)
	}
	if got[0].Error.AnCode != "unbekannt" {
		t.Errorf("got[0].Error = %+v", got[0].Error)
	}

	// Replaced, not appended.
	if err := pg.SetRegsWithErrors(ctx, want[:1]); err != nil {
		t.Fatalf("SetRegsWithErrors (second): %v", err)
	}
	got, err = pg.GetRegsWithErrors(ctx)
	if err != nil {
		t.Fatalf("GetRegsWithErrors: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1 -- the second write appended", len(got))
	}
}

func TestRegsWithErrorsEmptyIsNotNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetRegsWithErrors(t.Context())
	if err != nil {
		t.Fatalf("GetRegsWithErrors: %v", err)
	}
	if got == nil {
		t.Fatal("GetRegsWithErrors returned nil, want an empty slice")
	}
}
