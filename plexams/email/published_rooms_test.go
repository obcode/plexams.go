package email

import (
	"reflect"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func room(name string, students, duration int, reserve bool, ntaMtknr string) *model.PlannedRoom {
	r := &model.PlannedRoom{RoomName: name, Duration: duration, Reserve: reserve, StudentsInRoom: make([]string, students)}
	if ntaMtknr != "" {
		r.NtaMtknr = &ntaMtknr
	}
	return r
}

func TestRoomAllocationsOrdering(t *testing.T) {
	exam := &model.PlannedExam{
		Ntas: []*model.NTA{{Mtknr: "m1", Name: "Stud", DeltaDurationPercent: 25, NeedsRoomAlone: true}},
		PlannedRooms: []*model.PlannedRoom{
			room("R1", 20, 90, false, ""),   // non-NTA, 90
			room("R1", 1, 120, false, "m1"), // NTA, 120 -> last
			room("R1", 2, 90, true, ""),     // non-NTA reserve, 90 -> keeps input order after the first
			room("R2", 5, 90, false, ""),    // different room -> ignored
		},
	}
	got := roomAllocations(exam, "R1")
	want := []string{
		"20 Stud., 90 min",
		"2 Stud., 90 min (Reserve)",
		"1 Stud., 120 min, NTA: Stud (+25%, eigener Raum)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("roomAllocations =\n%v\nwant\n%v", got, want)
	}
}

func TestNtaNote(t *testing.T) {
	exam := &model.PlannedExam{Ntas: []*model.NTA{{Mtknr: "m1", Name: "Stud", DeltaDurationPercent: 50}}}
	if got := ntaNote(exam, room("R1", 1, 120, false, "m1")); got != "Stud (+50%)" {
		t.Errorf("ntaNote = %q", got)
	}
	// no NtaMtknr -> ""
	if got := ntaNote(exam, room("R1", 1, 90, false, "")); got != "" {
		t.Errorf("ntaNote (no NTA) = %q, want empty", got)
	}
}

var pubStart = time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)

func TestBuildPublishedRoomsExam(t *testing.T) {
	main := &model.PlannedExam{
		Ancode:    111,
		ZpaExam:   &model.ZPAExam{Module: "Mathe"},
		PlanEntry: &model.PlanEntry{Starttime: &pubStart},
		PlannedRooms: []*model.PlannedRoom{
			room("R1", 20, 90, false, ""),
			room("R1", 20, 90, false, ""), // same room again -> first-seen order, listed once
			room("R2", 3, 90, false, ""),
		},
	}
	other := &model.PlannedExam{
		Ancode:       222,
		ZpaExam:      &model.ZPAExam{Module: "Physik"},
		PlanEntry:    &model.PlanEntry{Starttime: &pubStart},
		PlannedRooms: []*model.PlannedRoom{room("R1", 5, 90, false, "")}, // shares R1
	}
	examsInSlot := map[time.Time][]*model.PlannedExam{pubStart: {main, other}}
	examerShort := func(e *model.PlannedExam) string {
		if e.Ancode == 222 {
			return "PB"
		}
		return "PA"
	}

	got := BuildPublishedRoomsExam(main, examsInSlot, examerShort)
	if got == nil {
		t.Fatal("BuildPublishedRoomsExam returned nil")
		return // unreachable, but pins got as non-nil for degraded-typecheck static analysis
	}
	if got.Ancode != 111 || got.Module != "Mathe" || got.Date != "Mo, 06.07.2026" || got.Time != "08:30" {
		t.Errorf("header = %+v", got)
	}
	// R1 (first-seen) then R2
	if len(got.Rooms) != 2 || got.Rooms[0].RoomName != "R1" || got.Rooms[1].RoomName != "R2" {
		t.Fatalf("rooms = %+v", got.Rooms)
	}
	// R1 shared with exam 222
	if len(got.Rooms[0].SharedWith) != 1 {
		t.Fatalf("R1 sharedWith = %+v", got.Rooms[0].SharedWith)
	}
	sw := got.Rooms[0].SharedWith[0]
	if sw.ExamHeader != "222. Physik (PB)" || !reflect.DeepEqual(sw.Allocations, []string{"5 Stud., 90 min"}) {
		t.Errorf("sharedWith = %+v", sw)
	}
	// R2 not shared
	if len(got.Rooms[1].SharedWith) != 0 {
		t.Errorf("R2 should not be shared: %+v", got.Rooms[1].SharedWith)
	}
}

func TestBuildPublishedRoomsExamNil(t *testing.T) {
	// no plan entry
	if got := BuildPublishedRoomsExam(&model.PlannedExam{Ancode: 1}, nil, nil); got != nil {
		t.Errorf("no plan entry -> want nil, got %+v", got)
	}
	// plan entry but no rooms
	noRooms := &model.PlannedExam{Ancode: 1, PlanEntry: &model.PlanEntry{Starttime: &pubStart}}
	if got := BuildPublishedRoomsExam(noRooms, map[time.Time][]*model.PlannedExam{}, nil); got != nil {
		t.Errorf("no rooms -> want nil, got %+v", got)
	}
}

func TestSortPublishedRoomsExams(t *testing.T) {
	early := &PublishedRoomsExam{Ancode: 1, start: time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)}
	late := &PublishedRoomsExam{Ancode: 2, start: time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)}
	exams := []*PublishedRoomsExam{late, early}
	SortPublishedRoomsExams(exams)
	if exams[0].Ancode != 1 || exams[1].Ancode != 2 {
		t.Errorf("sort order = %d, %d", exams[0].Ancode, exams[1].Ancode)
	}
}
