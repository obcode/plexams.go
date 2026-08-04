package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func seedSemester(t *testing.T, pg *db.PG) {
	t.Helper()
	exec(t, pg, `insert into semester (id, schema_version) values ('2026-WS', 2)`)
}

func testPreplanExam(id int, module string) *model.PreplanExam {
	return &model.PreplanExam{
		ID: id, ExamKind: "EXaHM", ExamerID: 180,
		ExamerName: "Prof. Dr. Oliver Braun", Module: module,
		Programs: []string{"DC"}, ExpectedStudents: 15,
	}
}

func TestInsertPreplanExamAssignsIDs(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	for i, module := range []string{"MPD", "Compilerbau", "Softwaretechnik"} {
		exam, err := pg.InsertPreplanExam(ctx, testPreplanExam(0, module))
		if err != nil {
			t.Fatalf("InsertPreplanExam(%s): %v", module, err)
		}
		if exam.ID != i+1 {
			t.Errorf("%s got id %d, want %d", module, exam.ID, i+1)
		}
	}

	// Deleting the last one and inserting again reuses the id -- max(id)+1 is
	// exactly what the Mongo version computed, gaps and all.
	if removed, err := pg.DeletePreplanExam(ctx, 3); err != nil || !removed {
		t.Fatalf("DeletePreplanExam: %v, removed=%v", err, removed)
	}
	exam, err := pg.InsertPreplanExam(ctx, testPreplanExam(0, "Wieder da"))
	if err != nil {
		t.Fatalf("InsertPreplanExam: %v", err)
	}
	if exam.ID != 3 {
		t.Errorf("id = %d, want 3", exam.ID)
	}
}

// The pair relations are symmetric and stored once, like exam_same_slot. Both
// sides read their own view of it.
func TestPreplanPairsAreSymmetric(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	for _, module := range []string{"A", "B", "C"} {
		if _, err := pg.InsertPreplanExam(ctx, testPreplanExam(0, module)); err != nil {
			t.Fatalf("InsertPreplanExam: %v", err)
		}
	}

	first := testPreplanExam(1, "A")
	first.NotSameSlot = []int{2}
	first.CanShareSlot = []int{3}
	if err := pg.UpsertPreplanExam(ctx, first); err != nil {
		t.Fatalf("UpsertPreplanExam: %v", err)
	}

	if n := count(t, pg, `select count(*) from preplan_not_same_slot where semester_id='2026-WS'`); n != 1 {
		t.Errorf("preplan_not_same_slot rows = %d, want 1", n)
	}

	second, err := pg.PreplanExam(ctx, 2)
	if err != nil {
		t.Fatalf("PreplanExam(2): %v", err)
	}
	if len(second.NotSameSlot) != 1 || second.NotSameSlot[0] != 1 {
		t.Errorf("NotSameSlot of 2 = %v, want [1] -- the pair is symmetric", second.NotSameSlot)
	}

	third, err := pg.PreplanExam(ctx, 3)
	if err != nil {
		t.Fatalf("PreplanExam(3): %v", err)
	}
	if len(third.CanShareSlot) != 1 || third.CanShareSlot[0] != 1 {
		t.Errorf("CanShareSlot of 3 = %v, want [1]", third.CanShareSlot)
	}

	// Replacing the first pre-exam without the pairs removes its whole side of
	// both relations -- the document-replacing semantics.
	if err := pg.UpsertPreplanExam(ctx, testPreplanExam(1, "A")); err != nil {
		t.Fatalf("UpsertPreplanExam: %v", err)
	}
	if n := count(t, pg, `select count(*) from preplan_not_same_slot where semester_id='2026-WS'`); n != 0 {
		t.Errorf("%d not-same-slot pairs left after the replace", n)
	}

	// And deleting a pre-exam takes its pairs with it.
	first.NotSameSlot = []int{2}
	if err := pg.UpsertPreplanExam(ctx, first); err != nil {
		t.Fatalf("UpsertPreplanExam: %v", err)
	}
	if _, err := pg.DeletePreplanExam(ctx, 2); err != nil {
		t.Fatalf("DeletePreplanExam: %v", err)
	}
	if n := count(t, pg, `select count(*) from preplan_not_same_slot where semester_id='2026-WS'`); n != 0 {
		t.Errorf("%d pairs left after the other pre-exam was deleted", n)
	}
}

// ReplacePreplanExam does not create -- that is the difference to
// UpsertPreplanExam, and the CSV round-trip depends on the latter keeping ids.
func TestReplaceAndUpsertPreplanExamDiffer(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	replaced, err := pg.ReplacePreplanExam(ctx, testPreplanExam(7, "Nicht da"))
	if err != nil {
		t.Fatalf("ReplacePreplanExam: %v", err)
	}
	if replaced {
		t.Error("ReplacePreplanExam reported a replacement of something that does not exist")
	}
	if n := count(t, pg, `select count(*) from preplan_exam where semester_id='2026-WS'`); n != 0 {
		t.Error("ReplacePreplanExam created a row")
	}

	// The CSV import keeps the id, so the pair references survive a restore.
	if err := pg.UpsertPreplanExam(ctx, testPreplanExam(7, "Aus der CSV")); err != nil {
		t.Fatalf("UpsertPreplanExam: %v", err)
	}
	exam, err := pg.PreplanExam(ctx, 7)
	if err != nil {
		t.Fatalf("PreplanExam: %v", err)
	}
	if exam == nil || exam.ID != 7 {
		t.Fatalf("PreplanExam(7) = %#v, want the imported id", exam)
	}

	exam.Module = "Umbenannt"
	replaced, err = pg.ReplacePreplanExam(ctx, exam)
	if err != nil || !replaced {
		t.Fatalf("ReplacePreplanExam: %v, replaced=%v", err, replaced)
	}
}

func TestPreplanExamRoundTripsItsFields(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	start := berlin(t, "2027-02-03 09:30")
	duration := 90
	ancode := 512
	exam := testPreplanExam(0, "MPD")
	exam.PlannedStarttime = &start
	exam.Duration = &duration
	exam.IsFixed = true
	exam.Ancode = &ancode
	exam.Notes = "im T-Bau"
	exam.Constraints = &model.Constraints{Ancode: 512, Online: true}

	if _, err := pg.InsertPreplanExam(ctx, exam); err != nil {
		t.Fatalf("InsertPreplanExam: %v", err)
	}

	got, err := pg.PreplanExam(ctx, 1)
	if err != nil {
		t.Fatalf("PreplanExam: %v", err)
	}
	if got.PlannedStarttime == nil || !got.PlannedStarttime.Equal(start) {
		t.Errorf("PlannedStarttime = %v, want %v", got.PlannedStarttime, start)
	}
	if *got.PlannedStarttime != start {
		t.Errorf("PlannedStarttime lost its location: %#v", *got.PlannedStarttime)
	}
	if got.Duration == nil || *got.Duration != duration {
		t.Errorf("Duration = %v, want %d", got.Duration, duration)
	}
	if got.Ancode == nil || *got.Ancode != ancode {
		t.Errorf("Ancode = %v, want %d", got.Ancode, ancode)
	}
	if got.Constraints == nil || !got.Constraints.Online {
		t.Errorf("Constraints = %#v, want the online flag back", got.Constraints)
	}
	if len(got.Programs) != 1 || got.Programs[0] != "DC" {
		t.Errorf("Programs = %v, want [DC] -- the raw ZPA group, which has no FK", got.Programs)
	}

	// A pre-exam that was never placed has no time; that is a real state, not a
	// missing value.
	if _, err := pg.InsertPreplanExam(ctx, testPreplanExam(0, "Noch offen")); err != nil {
		t.Fatalf("InsertPreplanExam: %v", err)
	}
	unplaced, err := pg.PreplanExam(ctx, 2)
	if err != nil {
		t.Fatalf("PreplanExam: %v", err)
	}
	if unplaced.PlannedStarttime != nil {
		t.Errorf("PlannedStarttime = %v, want nil", unplaced.PlannedStarttime)
	}
	if unplaced.Constraints != nil {
		t.Errorf("Constraints = %#v, want nil", unplaced.Constraints)
	}
}

func TestMissingPreplanExamIsNil(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	exam, err := pg.PreplanExam(ctx, 42)
	if err != nil {
		t.Fatalf("PreplanExam: %v", err)
	}
	if exam != nil {
		t.Errorf("PreplanExam = %#v, want nil", exam)
	}
	if removed, err := pg.DeletePreplanExam(ctx, 42); err != nil || removed {
		t.Errorf("DeletePreplanExam: %v, removed=%v, want false", err, removed)
	}
	exams, err := pg.PreplanExams(ctx)
	if err != nil {
		t.Fatalf("PreplanExams: %v", err)
	}
	if exams == nil || len(exams) != 0 {
		t.Errorf("PreplanExams = %v, want an empty slice", exams)
	}
}

func TestAnnyBookingsRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	start := berlin(t, "2027-02-03 09:30")
	booking := func(number, room string, offset time.Duration) *model.AnnyBooking {
		return &model.AnnyBooking{
			Number:           number,
			Room:             room,
			StartDate:        start.Add(offset),
			EndDate:          start.Add(offset + 2*time.Hour),
			BlockerStartDate: start.Add(offset - 15*time.Minute),
			BlockerEndDate:   start.Add(offset + 2*time.Hour + 15*time.Minute),
			CreatedAt:        start,
			UpdatedAt:        start,
			Status:           "confirmed",
			StatusReason:     map[string]any{"code": "ok"},
		}
	}

	if err := pg.SaveAnnyBookings(ctx, []*model.AnnyBooking{
		booking("B-2", "T3.017", time.Hour),
		booking("B-1", "R1.046", 0),
	}); err != nil {
		t.Fatalf("SaveAnnyBookings: %v", err)
	}

	all, err := pg.AllAnnyBookings(ctx)
	if err != nil {
		t.Fatalf("AllAnnyBookings: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d bookings, want 2", len(all))
	}
	if all[0].Number != "B-1" {
		t.Errorf("first booking = %s, want B-1 (sorted by start)", all[0].Number)
	}
	if !all[0].StartDate.Equal(start) || all[0].StartDate != start {
		t.Errorf("StartDate = %#v, want the struct-identical %#v", all[0].StartDate, start)
	}
	if all[0].StatusReason == nil {
		t.Error("StatusReason was lost")
	}

	// The room filter normalises the name, as the Mongo version did.
	room := " t3.017 "
	forRoom, err := pg.AnnyBookings(ctx, &room)
	if err != nil {
		t.Fatalf("AnnyBookings: %v", err)
	}
	if len(forRoom) != 1 || forRoom[0].Number != "B-2" {
		t.Errorf("AnnyBookings(%q) = %v, want only B-2", room, forRoom)
	}

	// A blank room is not a filter.
	blank := "  "
	if got, err := pg.AnnyBookings(ctx, &blank); err != nil {
		t.Fatalf("AnnyBookings: %v", err)
	} else if len(got) != 2 {
		t.Errorf("a blank room filtered %d bookings away", 2-len(got))
	}

	// Saving again replaces, it does not accumulate.
	if err := pg.SaveAnnyBookings(ctx, []*model.AnnyBooking{booking("B-3", "R1.046", 0)}); err != nil {
		t.Fatalf("SaveAnnyBookings: %v", err)
	}
	if all, err := pg.AllAnnyBookings(ctx); err != nil {
		t.Fatalf("AllAnnyBookings: %v", err)
	} else if len(all) != 1 || all[0].Number != "B-3" {
		t.Errorf("bookings after the second save = %v, want only B-3", all)
	}
}
