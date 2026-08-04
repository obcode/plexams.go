package main

import (
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/obcode/plexams.go/graph/model"
)

// TestMain pins time.Local, exactly as db/timezone_test.go does and for the same
// reason: BSON datetimes decode in UTC, everything downstream does day and slot
// arithmetic in Europe/Berlin, and the assertions here are about wall-clock
// times. Without this the tests would pass in the DevContainer and mean something
// different on a machine that is not on German time.
func TestMain(m *testing.M) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic("cannot load Europe/Berlin: " + err.Error())
	}
	time.Local = loc

	os.Exit(m.Run())
}

// berlin is a wall-clock time in the zone the planner thinks in.
func berlin(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

// The trap the schema comment in 00002 warns about: MucDaiAllowedTimes carries
// `json:"-"`, the column is jsonb, so writing the model as it stands drops the
// reserved times without a word. They have to become JointProgramAllowedTimes
// during the transfer -- one list, applied to every joint program, which is what
// applyLegacyJointTimes does at runtime.
func TestLegacyMucDaiTimesBecomeJointProgramTimes(t *testing.T) {
	d := decode(t, bson.M{
		"from":       berlin(t, "2026-07-06 00:00"),
		"until":      berlin(t, "2026-07-24 00:00"),
		"startTimes": bson.A{"08:30", "10:30"},
		"mucDaiAllowedTimes": bson.A{
			berlin(t, "2026-07-07 08:30"),
			berlin(t, "2026-07-09 10:30"),
		},
	})

	cfg, notes := convertSemesterConfigInput(d, []string{"DE", "GS", "ID"})

	if len(cfg.JointProgramAllowedTimes) != 3 {
		t.Fatalf("JointProgramAllowedTimes = %d Programme, want 3", len(cfg.JointProgramAllowedTimes))
	}
	for _, jpt := range cfg.JointProgramAllowedTimes {
		if len(jpt.AllowedTimes) != 2 {
			t.Errorf("%s: %d reservierte Zeiten, want 2", jpt.Program, len(jpt.AllowedTimes))
		}
	}
	if len(notes) == 0 {
		t.Error("the conversion must be reported -- it is the one thing a reader cannot see in the result")
	}
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("mucDaiAllowedTimes must count as consumed, leftovers = %v", left)
	}
}

// A configuration that already has per-program times is left alone: the legacy
// list is then a leftover of an earlier edit, not the live data.
func TestPerProgramTimesWinOverTheLegacyList(t *testing.T) {
	d := decode(t, bson.M{
		"from": berlin(t, "2026-07-06 00:00"), "until": berlin(t, "2026-07-24 00:00"),
		"startTimes": bson.A{"08:30"},
		"jointProgramAllowedTimes": bson.A{
			bson.M{"program": "DE", "allowedTimes": bson.A{berlin(t, "2026-07-07 08:30")}},
		},
		"mucDaiAllowedTimes": bson.A{berlin(t, "2026-07-20 08:30")},
	})

	cfg, _ := convertSemesterConfigInput(d, []string{"DE", "GS"})

	if len(cfg.JointProgramAllowedTimes) != 1 || cfg.JointProgramAllowedTimes[0].Program != "DE" {
		t.Fatalf("JointProgramAllowedTimes = %+v, want only the stored DE entry", cfg.JointProgramAllowedTimes)
	}
}

// Times must arrive in Europe/Berlin. The driver hands out UTC, and a time with a
// UTC location is a DIFFERENT map key for the same instant -- the failure the db
// layer pins from the other side with TestTimestamptzKeepsLocation.
func TestConfigTimesArriveInLocalTime(t *testing.T) {
	from := berlin(t, "2026-07-06 00:00")
	d := decode(t, bson.M{
		"from": from, "until": berlin(t, "2026-07-24 00:00"),
		"startTimes":    bson.A{"08:30"},
		"forbiddenDays": bson.A{berlin(t, "2026-07-15 00:00")},
	})

	cfg, _ := convertSemesterConfigInput(d, nil)

	if !cfg.From.Equal(from) {
		t.Errorf("From = %v, want the same instant as %v", cfg.From, from)
	}
	if _, offset := cfg.From.Zone(); offset == 0 {
		t.Errorf("From carries %v, want Europe/Berlin", cfg.From.Location())
	}
	if len(cfg.ForbiddenDays) != 1 || cfg.ForbiddenDays[0].Day() != 15 {
		t.Errorf("ForbiddenDays = %v, want the 15th in local time", cfg.ForbiddenDays)
	}
}

// The emails sub-document was stored with lowercased Go field names, because the
// generated model has no bson tags. And an unknown key inside it has to surface
// as a leftover just like a top-level one -- that is what sub-document tracking
// is for.
func TestEmailsAreReadFromTheLowercasedKeys(t *testing.T) {
	d := decode(t, bson.M{
		"from": berlin(t, "2026-07-06 00:00"), "until": berlin(t, "2026-07-24 00:00"),
		"startTimes": bson.A{"08:30"},
		"emails": bson.M{
			"profs": "profs@example.org", "lbaslastsemester": "alt@example.org",
			"roommanagement": "gm@example.org", "additionalexamer": bson.A{"x@example.org"},
			"bcc": "vergessen@example.org",
		},
	})

	cfg, _ := convertSemesterConfigInput(d, nil)

	if cfg.Emails.Profs != "profs@example.org" || cfg.Emails.LbasLastSemester != "alt@example.org" ||
		cfg.Emails.RoomManagement != "gm@example.org" || len(cfg.Emails.AdditionalExamer) != 1 {
		t.Errorf("Emails = %+v", cfg.Emails)
	}
	left := d.leftovers()
	if len(left) != 1 || left[0] != "emails.bcc" {
		t.Errorf("leftovers = %v, want [emails.bcc]", left)
	}
}

// A pre-slotless configuration cannot yield start times: the day/slot ordinals
// were mapped to times by the code of that version, not by the document. Saying
// so is the whole job here -- a config with no start times plans nothing.
func TestPreSlotlessConfigIsReportedNotGuessed(t *testing.T) {
	d := decode(t, bson.M{
		"from": berlin(t, "2026-01-26 00:00"), "until": berlin(t, "2026-02-13 00:00"),
		"slots":       bson.A{bson.A{1, 1}, bson.A{1, 2}},
		"mucDaiSlots": bson.A{bson.A{2, 1}},
	})

	cfg, notes := convertSemesterConfigInput(d, []string{"DE"})

	if len(cfg.StartTimes) != 0 {
		t.Errorf("StartTimes = %v, want none invented", cfg.StartTimes)
	}
	if len(notes) < 2 {
		t.Errorf("the old shape AND the missing start times must both be reported, got %v", notes)
	}
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("both legacy keys must count as consumed, leftovers = %v", left)
	}
}

// A current pre-exam: the placement is an absolute time, and the constraints
// sub-document comes along.
func TestPreplanExamKeepsItsAbsoluteTime(t *testing.T) {
	start := berlin(t, "2026-07-08 10:30")
	d := decode(t, bson.M{
		"id": 3, "examkind": "EXaHM", "examerid": 123, "examername": "Test",
		"module": "Modul", "programs": bson.A{"IF", "IB"}, "expectedstudents": 40,
		"duration": 90, "plannedstarttime": start, "isfixed": true,
		"constraints": bson.M{"online": false, "notplannedbyme": false, "donotpublish": true,
			"sameslot": bson.A{4}},
	})

	e, notes := convertPreplanExam(d)

	if e.PlannedStarttime == nil || !e.PlannedStarttime.Equal(start) {
		t.Fatalf("PlannedStarttime = %v, want %v", e.PlannedStarttime, start)
	}
	if _, offset := e.PlannedStarttime.Zone(); offset == 0 {
		t.Errorf("PlannedStarttime carries %v, want Europe/Berlin", e.PlannedStarttime.Location())
	}
	if e.Constraints == nil || !e.Constraints.DoNotPublish || len(e.Constraints.SameSlot) != 1 {
		t.Errorf("Constraints = %+v", e.Constraints)
	}
	if len(notes) != 0 {
		t.Errorf("a current document needs no notes, got %v", notes)
	}
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("unexpected leftovers %v", left)
	}
}

// A pre-slotless pre-exam is imported unplaced, with a note. Everything else
// about it -- module, examiner, expected students -- is still valid, so dropping
// the exam over its placement would cost more than it saves.
func TestPreSlotlessPreplanExamComesInUnplanned(t *testing.T) {
	d := decode(t, bson.M{
		"id": 7, "examkind": "SEB", "examerid": 1, "examername": "Test", "module": "M",
		"programs": bson.A{"IF"}, "expectedstudents": 10,
		"planneddaynumber": 3, "plannedslotnumber": 2,
	})

	e, notes := convertPreplanExam(d)

	if e.ID != 7 || e.ExamKind != "SEB" {
		t.Errorf("the exam itself must survive, got %+v", e)
	}
	if e.PlannedStarttime != nil {
		t.Errorf("PlannedStarttime = %v, want nil -- the time is not recoverable", e.PlannedStarttime)
	}
	if len(notes) != 1 {
		t.Errorf("the dropped placement must be reported once, got %v", notes)
	}
	if left := d.leftovers(); len(left) != 0 {
		t.Errorf("the day/slot keys must count as consumed, leftovers = %v", left)
	}
}

// In MongoDB nothing forced the two sides of a pair to agree. In PostgreSQL a
// pair is one canonical row, and writing an exam replaces its whole side of the
// relation -- so a one-sided pair would be deleted again by the write of the
// other exam. Completing it first makes the result independent of the order.
func TestOneSidedPairsAreCompleted(t *testing.T) {
	exams := []*model.PreplanExam{
		{ID: 1, NotSameSlot: []int{2}},
		{ID: 2},
		{ID: 3, CanShareSlot: []int{1}},
	}

	notes := makePairsSymmetric(exams)

	if len(exams[1].NotSameSlot) != 1 || exams[1].NotSameSlot[0] != 1 {
		t.Errorf("#2 NotSameSlot = %v, want [1]", exams[1].NotSameSlot)
	}
	if len(exams[0].CanShareSlot) != 1 || exams[0].CanShareSlot[0] != 3 {
		t.Errorf("#1 CanShareSlot = %v, want [3]", exams[0].CanShareSlot)
	}
	if len(notes) != 2 {
		t.Errorf("both completions must be reported, got %v", notes)
	}
}

// A pair pointing at a pre-exam that is not in the dump would be rejected by the
// foreign key, taking the whole import with it. It is dropped -- and said aloud.
func TestPairsToUnknownExamsAreDroppedWithANote(t *testing.T) {
	exams := []*model.PreplanExam{{ID: 1, NotSameSlot: []int{99}}}

	notes := makePairsSymmetric(exams)

	if len(exams[0].NotSameSlot) != 0 {
		t.Errorf("NotSameSlot = %v, want empty", exams[0].NotSameSlot)
	}
	if len(notes) != 1 {
		t.Fatalf("the dropped pair must be reported, got %v", notes)
	}
}
