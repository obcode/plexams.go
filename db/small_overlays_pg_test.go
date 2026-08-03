package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/zpa"
)

func TestExamDurationOverrides(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100, 200)

	if _, err := pg.SetExamDurationOverride(ctx, 100, 120); err != nil {
		t.Fatalf("SetExamDurationOverride: %v", err)
	}
	if _, err := pg.SetExamDurationOverride(ctx, 100, 150); err != nil {
		t.Fatalf("SetExamDurationOverride again: %v", err)
	}
	if _, err := pg.SetExamDurationOverride(ctx, 200, 60); err != nil {
		t.Fatalf("SetExamDurationOverride: %v", err)
	}

	overrides, err := pg.ExamDurationOverrides(ctx)
	if err != nil {
		t.Fatalf("ExamDurationOverrides: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("got %d overrides, want 2", len(overrides))
	}
	if overrides[0].Ancode != 100 || overrides[0].Duration != 150 {
		t.Errorf("override = %#v, want ancode 100 at 150 minutes", overrides[0])
	}

	if removed, err := pg.RemoveExamDurationOverride(ctx, 100); err != nil || !removed {
		t.Fatalf("RemoveExamDurationOverride: %v, removed=%v", err, removed)
	}
	if removed, err := pg.RemoveExamDurationOverride(ctx, 100); err != nil || removed {
		t.Errorf("RemoveExamDurationOverride again: %v, removed=%v, want false", err, removed)
	}
}

// An override for an exam that does not exist is rejected -- that is the
// dangling-ancode report in plexams/validate_db.go:383 becoming a constraint.
func TestADurationOverrideNeedsItsExam(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg)

	if _, err := pg.SetExamDurationOverride(ctx, 999, 90); err == nil {
		t.Error("an override for an exam that does not exist was accepted")
	}
}

func TestAdditionalExamsCarryTheirRooms(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60), ('R1.006', 30)`)

	if err := pg.UpsertAdditionalExam(ctx, &model.AdditionalExam{
		Ancode: 100, Date: "20.01.2027", Time: "08:30",
		Rooms: []*model.AdditionalExamRoom{
			{RoomName: "R1.046", InvigilatorID: 180, Duration: 90, StudentCount: 40},
			{RoomName: "R1.006", InvigilatorID: 181, Duration: 90, IsReserve: true},
		},
	}); err != nil {
		t.Fatalf("UpsertAdditionalExam: %v", err)
	}

	exams, err := pg.AdditionalExams(ctx)
	if err != nil {
		t.Fatalf("AdditionalExams: %v", err)
	}
	if len(exams) != 1 || len(exams[0].Rooms) != 2 {
		t.Fatalf("exams = %#v, want one exam with two rooms", exams)
	}
	if exams[0].Rooms[0].RoomName != "R1.006" {
		t.Errorf("rooms are not sorted: %v", exams[0].Rooms[0].RoomName)
	}

	// Upserting again replaces the rooms rather than adding to them -- they were
	// an array inside the document.
	if err := pg.UpsertAdditionalExam(ctx, &model.AdditionalExam{
		Ancode: 100, Date: "21.01.2027", Time: "10:00",
		Rooms: []*model.AdditionalExamRoom{
			{RoomName: "R1.046", InvigilatorID: 180, Duration: 120, StudentCount: 40},
		},
	}); err != nil {
		t.Fatalf("UpsertAdditionalExam: %v", err)
	}
	exams, err = pg.AdditionalExams(ctx)
	if err != nil {
		t.Fatalf("AdditionalExams: %v", err)
	}
	if len(exams[0].Rooms) != 1 || exams[0].Date != "21.01.2027" {
		t.Errorf("exam after the second upsert = %#v", exams[0])
	}

	if removed, err := pg.DeleteAdditionalExam(ctx, 100); err != nil || !removed {
		t.Fatalf("DeleteAdditionalExam: %v, removed=%v", err, removed)
	}
	if n := count(t, pg, `select count(*) from additional_exam_room where semester_id='2026-WS'`); n != 0 {
		t.Errorf("%d rooms left after the exam was deleted", n)
	}

	// An exam with no rooms comes back with an empty slice, not nil.
	if err := pg.UpsertAdditionalExam(ctx, &model.AdditionalExam{Ancode: 100}); err != nil {
		t.Fatalf("UpsertAdditionalExam: %v", err)
	}
	exams, err = pg.AdditionalExams(ctx)
	if err != nil {
		t.Fatalf("AdditionalExams: %v", err)
	}
	if exams[0].Rooms == nil {
		t.Error("Rooms is nil, want an empty slice")
	}
}

func TestSpecialInterests(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	if err := pg.UpsertSpecialInterest(ctx, &model.SpecialInterest{
		Name: "Wahlpflicht", Filename: "wahlpflicht.pdf", Ancodes: []int{100, 200},
	}); err != nil {
		t.Fatalf("UpsertSpecialInterest: %v", err)
	}
	// The ancodes have no foreign key on purpose: this is a display grouping for
	// a report, and a stale entry costs a missing line, not a wrong plan. Neither
	// exam exists here.
	if err := pg.UpsertSpecialInterest(ctx, &model.SpecialInterest{
		Name: "Abschluss", Filename: "abschluss.pdf",
	}); err != nil {
		t.Fatalf("UpsertSpecialInterest: %v", err)
	}

	sis, err := pg.SpecialInterests(ctx)
	if err != nil {
		t.Fatalf("SpecialInterests: %v", err)
	}
	if len(sis) != 2 || sis[0].Name != "Abschluss" {
		t.Fatalf("special interests = %#v, want two, sorted by name", sis)
	}
	if sis[0].Ancodes == nil {
		t.Error("Ancodes is nil, want an empty slice")
	}
	if len(sis[1].Ancodes) != 2 {
		t.Errorf("Ancodes = %v, want both", sis[1].Ancodes)
	}

	if removed, err := pg.DeleteSpecialInterest(ctx, "Abschluss"); err != nil || !removed {
		t.Fatalf("DeleteSpecialInterest: %v, removed=%v", err, removed)
	}
	if removed, err := pg.DeleteSpecialInterest(ctx, "Abschluss"); err != nil || removed {
		t.Errorf("DeleteSpecialInterest again: %v, removed=%v, want false", err, removed)
	}
}

func TestNtaRoomAloneWaivers(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 530)
	exec(t, pg, `insert into nta (mtknr, name, compensation, delta_duration_percent,
	                              program, valid_from, valid_until)
	             values ('51490923', 'Eine Person', 'Zeitverlängerung', 10, 'IF-B',
	                     '2026-WS', '2027-SS')`)

	waiver := &model.NtaRoomAloneWaiver{
		Mtknr: "51490923", Ancode: 530, Reason: "gemäß Ihrer E-Mail vom 09.06.26",
	}
	if err := pg.AddNtaRoomAloneWaiver(ctx, waiver); err != nil {
		t.Fatalf("AddNtaRoomAloneWaiver: %v", err)
	}
	// Storing it again replaces the reason, as ReplaceOne with upsert did.
	waiver.Reason = "neu begründet"
	if err := pg.AddNtaRoomAloneWaiver(ctx, waiver); err != nil {
		t.Fatalf("AddNtaRoomAloneWaiver again: %v", err)
	}

	waivers, err := pg.NtaRoomAloneWaivers(ctx)
	if err != nil {
		t.Fatalf("NtaRoomAloneWaivers: %v", err)
	}
	if len(waivers) != 1 || waivers[0].Reason != "neu begründet" {
		t.Fatalf("waivers = %#v, want one with the new reason", waivers)
	}

	if removed, err := pg.RemoveNtaRoomAloneWaiver(ctx, "51490923", 530); err != nil || !removed {
		t.Fatalf("RemoveNtaRoomAloneWaiver: %v, removed=%v", err, removed)
	}
	if removed, err := pg.RemoveNtaRoomAloneWaiver(ctx, "51490923", 530); err != nil || removed {
		t.Errorf("RemoveNtaRoomAloneWaiver again: %v, removed=%v, want false", err, removed)
	}
}

// A waiver exempts a student from an NTA entitlement, so there has to be one --
// and the exam has to exist too.
func TestAWaiverNeedsAnNtaAndAnExam(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 530)

	if err := pg.AddNtaRoomAloneWaiver(ctx, &model.NtaRoomAloneWaiver{
		Mtknr: "00000000", Ancode: 530, Reason: "kein NTA",
	}); err == nil {
		t.Error("a waiver for a student with no NTA entitlement was accepted")
	}
}

func TestReplaceAllIsTypedPerTarget(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	if err := pg.ReplaceAll(ctx, db.TargetZPAStudents, []interface{}{
		&model.ZPAStudent{Mtknr: "00000001", FirstName: "A", LastName: "B", Group: "IF4B"},
		&model.ZPAStudent{Mtknr: "00000002", FirstName: "C", LastName: "D", Group: "IF4B"},
	}); err != nil {
		t.Fatalf("ReplaceAll(students): %v", err)
	}
	students, err := pg.GetZPAStudents(ctx)
	if err != nil {
		t.Fatalf("GetZPAStudents: %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("got %d students, want 2", len(students))
	}

	// Replacing with fewer clears the rest.
	if err := pg.ReplaceAll(ctx, db.TargetZPAStudents, []interface{}{
		&model.ZPAStudent{Mtknr: "00000003", FirstName: "E", LastName: "F"},
	}); err != nil {
		t.Fatalf("ReplaceAll(students): %v", err)
	}
	if students, err := pg.GetZPAStudents(ctx); err != nil {
		t.Fatalf("GetZPAStudents: %v", err)
	} else if len(students) != 1 || students[0].Mtknr != "00000003" {
		t.Errorf("students after the second replace = %v", students)
	}

	// An empty slice clears the target.
	if err := pg.ReplaceAll(ctx, db.TargetZPAStudents, nil); err != nil {
		t.Fatalf("ReplaceAll(nil): %v", err)
	}
	if students, err := pg.GetZPAStudents(ctx); err != nil {
		t.Fatalf("GetZPAStudents: %v", err)
	} else if len(students) != 0 {
		t.Errorf("%d students left after an empty replace", len(students))
	}

	// The wrong type for a target fails loudly instead of writing something
	// nobody can read back -- which is what the untyped Mongo version did.
	if err := pg.ReplaceAll(ctx, db.TargetZPAStudents, []interface{}{
		&zpa.SupervisorRequirements{InvigilatorID: 180},
	}); err == nil {
		t.Error("an object of the wrong type was accepted for TargetZPAStudents")
	}

	if err := pg.ReplaceAll(ctx, db.TargetInvigilatorRequirements, []interface{}{
		&zpa.SupervisorRequirements{
			Invigilator: "Braun", InvigilatorID: 180, PartTime: 1,
			ExcludedDates: []string{"20.01.2027"},
		},
	}); err != nil {
		t.Fatalf("ReplaceAll(requirements): %v", err)
	}
	if n := count(t, pg, `select count(*) from invigilator_requirement where semester_id='2026-WS'`); n != 1 {
		t.Errorf("invigilator_requirement rows = %d, want 1", n)
	}
}

// The two invigilation targets are one table told apart by a flag, so each must
// clear only its own half. Getting this wrong would have made generating the
// other invigilations delete the self-invigilations.
func TestTheTwoInvigilationTargetsDoNotClearEachOther(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)
	exec(t, pg, `insert into room (name, seats) values ('R1.046', 60)`)

	start := berlin(t, "2027-01-20 08:30")
	room := "R1.046"

	// A self-invigilation is stored with duration 0 deliberately: it must not
	// count towards the invigilator's duty (invigcalc.Todos excludes it by the
	// flag). All 43 in 2026-SS carry 0, which is why the check is >= 0.
	if err := pg.ReplaceAll(ctx, db.TargetSelfInvigilations, []interface{}{
		&model.Invigilation{InvigilatorID: 180, Starttime: &start, Duration: 0},
	}); err != nil {
		t.Fatalf("ReplaceAll(self): %v", err)
	}
	if n := count(t, pg, `select count(*) from invigilation
	                      where semester_id='2026-WS' and is_self_invigilation
	                        and duration_min = 0`); n != 1 {
		t.Errorf("the self-invigilation did not keep its duration 0 -- it would "+
			"start counting towards the todos (%d rows with 0)", n)
	}
	if err := pg.ReplaceAll(ctx, db.TargetOtherInvigilations, []interface{}{
		&model.Invigilation{InvigilatorID: 181, Starttime: &start, Duration: 90, RoomName: &room},
		&model.Invigilation{InvigilatorID: 182, Starttime: &start, Duration: 90, IsReserve: true},
	}); err != nil {
		t.Fatalf("ReplaceAll(other): %v", err)
	}

	if n := count(t, pg, `select count(*) from invigilation
	                      where semester_id='2026-WS' and is_self_invigilation`); n != 1 {
		t.Errorf("%d self-invigilations left, want 1 -- the other target cleared them", n)
	}
	if n := count(t, pg, `select count(*) from invigilation
	                      where semester_id='2026-WS' and not is_self_invigilation`); n != 2 {
		t.Errorf("%d other invigilations, want 2", n)
	}

	// An invigilation without a start time is refused rather than stored: the
	// column is the whole key to when it happens.
	if err := pg.ReplaceAll(ctx, db.TargetOtherInvigilations, []interface{}{
		&model.Invigilation{InvigilatorID: 181, Duration: 90},
	}); err == nil {
		t.Error("an invigilation without a starttime was accepted")
	}
}

func TestReplaceAllRejectsAnUnknownTarget(t *testing.T) {
	pg := pgtest.NewDB(t)
	seedSemester(t, pg)

	if err := pg.ReplaceAll(t.Context(), db.ReplaceTarget("nonsense"), nil); err == nil {
		t.Error("an unknown replace target was accepted")
	}
}
