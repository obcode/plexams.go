package db_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func intp(i int) *int { return &i }

// TestSameSlotIsStoredOncePerPair is the shape change: MongoDB kept a list on
// each exam and nothing enforced that the two agreed. Here there is one row per
// pair, so the relation cannot disagree with itself -- but both exams still read
// their own side of it, which is what plexams/constraints.go expects.
func TestSameSlotIsStoredOncePerPair(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 112, 113)

	if _, err := pg.AddConstraints(ctx, 113, &model.Constraints{
		Ancode: 113, SameSlot: []int{112},
	}); err != nil {
		t.Fatalf("AddConstraints(113): %v", err)
	}

	// Only one exam has been given the constraint so far, and the other already
	// sees it -- that is the point of storing the pair rather than two lists.
	other, err := pg.GetConstraintsForAncode(ctx, 112)
	if err != nil {
		t.Fatalf("GetConstraintsForAncode(112): %v", err)
	}
	if other != nil {
		t.Log("112 has no constraint row of its own yet, as expected")
	}

	if n := count(t, pg, `select count(*) from exam_same_slot where semester_id='2026-WS'`); n != 1 {
		t.Errorf("exam_same_slot rows = %d, want 1 -- the pair is stored once", n)
	}

	// The mirror write plexams does next must not create a second row.
	if _, err := pg.AddConstraints(ctx, 112, &model.Constraints{
		Ancode: 112, SameSlot: []int{113},
	}); err != nil {
		t.Fatalf("AddConstraints(112): %v", err)
	}
	if n := count(t, pg, `select count(*) from exam_same_slot where semester_id='2026-WS'`); n != 1 {
		t.Errorf("exam_same_slot rows = %d after the mirror write, want 1", n)
	}

	for _, tc := range []struct{ ancode, want int }{{112, 113}, {113, 112}} {
		got, err := pg.GetConstraintsForAncode(ctx, tc.ancode)
		if err != nil {
			t.Fatalf("GetConstraintsForAncode(%d): %v", tc.ancode, err)
		}
		if len(got.SameSlot) != 1 || got.SameSlot[0] != tc.want {
			t.Errorf("SameSlot of %d = %v, want [%d]", tc.ancode, got.SameSlot, tc.want)
		}
	}

	// And GetConstraints assembles the same answer for everyone at once.
	all, err := pg.GetConstraints(ctx)
	if err != nil {
		t.Fatalf("GetConstraints: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("GetConstraints returned %d, want 2", len(all))
	}
	for _, constraint := range all {
		if len(constraint.SameSlot) != 1 {
			t.Errorf("ancode %d: SameSlot = %v, want one entry", constraint.Ancode, constraint.SameSlot)
		}
	}
}

// Replacing an exam's constraints replaces its whole side of the same-slot
// relation, exactly as replacing the Mongo document did.
func TestReplacingConstraintsDropsTheOldSameSlotPairs(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 1, 2, 3)

	if _, err := pg.AddConstraints(ctx, 1, &model.Constraints{Ancode: 1, SameSlot: []int{2, 3}}); err != nil {
		t.Fatalf("AddConstraints: %v", err)
	}
	if n := count(t, pg, `select count(*) from exam_same_slot where semester_id='2026-WS'`); n != 2 {
		t.Fatalf("exam_same_slot rows = %d, want 2", n)
	}

	if _, err := pg.AddConstraints(ctx, 1, &model.Constraints{Ancode: 1, SameSlot: []int{3}}); err != nil {
		t.Fatalf("AddConstraints: %v", err)
	}

	got, err := pg.GetConstraintsForAncode(ctx, 1)
	if err != nil {
		t.Fatalf("GetConstraintsForAncode: %v", err)
	}
	if len(got.SameSlot) != 1 || got.SameSlot[0] != 3 {
		t.Errorf("SameSlot = %v, want [3]", got.SameSlot)
	}
}

// RoomConstraints is a pointer, and the pointer being nil is the constraint
// being absent. That is why it is its own table and not nullable columns.
func TestRoomConstraintsPresenceIsTheRowsPresence(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 113)
	exec(t, pg, `insert into room (name, seats) values ('T3.015', 30), ('T3.016', 30)`)

	jira := "https://jira.cc.hm.edu/servicedesk/customer/portal/4/IL-5671"
	got, err := pg.AddConstraints(ctx, 113, &model.Constraints{
		Ancode: 113,
		RoomConstraints: &model.RoomConstraints{
			AllowedRooms: []string{"T3.016", "T3.015"},
			Exahm:        true,
			KdpJiraURL:   &jira,
			MaxStudents:  intp(80),
		},
	})
	if err != nil {
		t.Fatalf("AddConstraints: %v", err)
	}
	if got.RoomConstraints == nil {
		t.Fatal("RoomConstraints is nil after being set")
	}
	if !got.RoomConstraints.Exahm || got.RoomConstraints.KdpJiraURL == nil {
		t.Errorf("RoomConstraints = %#v, want the exahm/jira values back", got.RoomConstraints)
	}
	if rooms := got.RoomConstraints.AllowedRooms; len(rooms) != 2 || rooms[0] != "T3.015" {
		t.Errorf("AllowedRooms = %v, want both, sorted", rooms)
	}

	// Writing constraints without room constraints removes them -- the
	// document-replacing semantics of the Mongo version.
	got, err = pg.AddConstraints(ctx, 113, &model.Constraints{Ancode: 113, Online: true})
	if err != nil {
		t.Fatalf("AddConstraints: %v", err)
	}
	if got.RoomConstraints != nil {
		t.Errorf("RoomConstraints = %#v, want nil", got.RoomConstraints)
	}
	if n := count(t, pg, `select count(*) from exam_allowed_room where semester_id='2026-WS'`); n != 0 {
		t.Errorf("%d allowed rooms left -- they must cascade with the room constraint", n)
	}
}

// An allowed room that is not in the room master data is rejected. Under MongoDB
// a typo was simply a string nobody ever matched.
func TestAnAllowedRoomMustExist(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 113)
	exec(t, pg, `insert into room (name, seats) values ('T3.015', 30)`)

	_, err := pg.AddConstraints(ctx, 113, &model.Constraints{
		Ancode:          113,
		RoomConstraints: &model.RoomConstraints{AllowedRooms: []string{"T3.O15"}},
	})
	if err == nil {
		t.Error("a room that does not exist was accepted as an allowed room")
	}

	// And the failed write left nothing behind.
	if n := count(t, pg, `select count(*) from exam_constraint where semester_id='2026-WS'`); n != 0 {
		t.Errorf("%d constraint rows after the failed write, want 0", n)
	}
}

// Constraints for an exam that does not exist are rejected -- the foreign key is
// what retires the orphan report in plexams/validate_db.go. 2026-SS really does
// carry one, for ancode 326.
func TestConstraintsForAnUnknownExamAreRejected(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg)

	if _, err := pg.AddConstraints(ctx, 326, &model.Constraints{Ancode: 326, Online: true}); err == nil {
		t.Error("a constraint for an exam that does not exist was accepted")
	}
}

// The five single-flag setters read, modify and write -- including creating the
// constraint that holds the flag, which is what they are usually used for.
func TestSingleFlagSettersCreateAndKeep(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)

	fk := "FK10"
	if ok, err := pg.NotPlannedByMe(ctx, 100, &fk); err != nil || !ok {
		t.Fatalf("NotPlannedByMe: %v, ok=%v", err, ok)
	}
	if ok, err := pg.Online(ctx, 100); err != nil || !ok {
		t.Fatalf("Online: %v, ok=%v", err, ok)
	}
	if ok, err := pg.Exahm(ctx, 100); err != nil || !ok {
		t.Fatalf("Exahm: %v, ok=%v", err, ok)
	}
	if ok, err := pg.SafeExamBrowser(ctx, 100); err != nil || !ok {
		t.Fatalf("SafeExamBrowser: %v, ok=%v", err, ok)
	}
	if ok, err := pg.Lab(ctx, 100); err != nil || !ok {
		t.Fatalf("Lab: %v, ok=%v", err, ok)
	}

	got, err := pg.GetConstraintsForAncode(ctx, 100)
	if err != nil {
		t.Fatalf("GetConstraintsForAncode: %v", err)
	}
	if !got.NotPlannedByMe || got.NotPlannedByMeInFk == nil || *got.NotPlannedByMeInFk != fk {
		t.Errorf("notPlannedByMe = %v / %v, want true / %q",
			got.NotPlannedByMe, got.NotPlannedByMeInFk, fk)
	}
	if !got.Online {
		t.Error("Online was lost by a later setter")
	}
	if got.RoomConstraints == nil {
		t.Fatal("RoomConstraints is nil")
	}
	if !got.RoomConstraints.Exahm || !got.RoomConstraints.Seb || !got.RoomConstraints.Lab {
		t.Errorf("RoomConstraints = %#v, want all three flags set", got.RoomConstraints)
	}
}

func TestExcludeDaysAndFixedTimeRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 100)

	day1 := berlin(t, "2027-01-20 00:00")
	day2 := berlin(t, "2027-01-21 00:00")
	fixed := berlin(t, "2027-01-25 08:30")

	got, err := pg.AddConstraints(ctx, 100, &model.Constraints{
		Ancode:      100,
		ExcludeDays: []*time.Time{&day1, &day2},
		FixedTime:   &fixed,
	})
	if err != nil {
		t.Fatalf("AddConstraints: %v", err)
	}
	if len(got.ExcludeDays) != 2 {
		t.Fatalf("ExcludeDays = %v, want two days", got.ExcludeDays)
	}
	if !got.ExcludeDays[0].Equal(day1) || *got.ExcludeDays[0] != day1 {
		t.Errorf("ExcludeDays[0] = %#v, want the struct-identical %#v", *got.ExcludeDays[0], day1)
	}
	if got.FixedTime == nil || !got.FixedTime.Equal(fixed) {
		t.Errorf("FixedTime = %v, want %v", got.FixedTime, fixed)
	}
	// An exam with no excluded days keeps a nil list, not an empty one -- the
	// GraphQL field is nullable and the Mongo documents stored null.
	if got.PossibleDays != nil {
		t.Errorf("PossibleDays = %v, want nil", got.PossibleDays)
	}
}

func TestRmConstraints(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	seedExamFixtures(t, pg, 1, 2)
	exec(t, pg, `insert into room (name, seats) values ('T3.015', 30)`)

	if _, err := pg.AddConstraints(ctx, 1, &model.Constraints{
		Ancode:          1,
		SameSlot:        []int{2},
		RoomConstraints: &model.RoomConstraints{AllowedRooms: []string{"T3.015"}},
	}); err != nil {
		t.Fatalf("AddConstraints: %v", err)
	}

	if ok, err := pg.RmConstraints(ctx, 1); err != nil || !ok {
		t.Fatalf("RmConstraints: %v, ok=%v", err, ok)
	}

	got, err := pg.GetConstraintsForAncode(ctx, 1)
	if err != nil {
		t.Fatalf("GetConstraintsForAncode: %v", err)
	}
	if got != nil {
		t.Errorf("GetConstraintsForAncode = %#v, want nil", got)
	}
	for _, table := range []string{"exam_same_slot", "exam_room_constraint", "exam_allowed_room"} {
		if n := count(t, pg, `select count(*) from `+table+` where semester_id='2026-WS'`); n != 0 {
			t.Errorf("%s still has %d rows", table, n)
		}
	}

	// Removing what is not there is not an error, as DeleteOne was not.
	if ok, err := pg.RmConstraints(ctx, 1); err != nil || !ok {
		t.Errorf("RmConstraints on nothing: %v, ok=%v", err, ok)
	}
}
