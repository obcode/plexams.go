package plexams

import (
	"context"
	"testing"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/mongotest"
)

// Characterization test for the stateful room-allocation state machine
// (prepareRoomsForExamsInSlot). It runs against an ephemeral MongoDB
// (PLEXAMS_TEST_MONGO_URI or testcontainers) and is skipped when neither is available.
// It pins the current behaviour before any future extraction of the roomsPrepare cfg
// machine into its own package.

// studentRegs returns n synthetic registrations with Matrikelnummern prefix+"0".. .
func studentRegs(prefix string, n int) []*model.EnhancedStudentReg {
	regs := make([]*model.EnhancedStudentReg, n)
	for i := 0; i < n; i++ {
		regs[i] = &model.EnhancedStudentReg{Mtknr: mtknr(prefix, i)}
	}
	return regs
}

func mtknr(prefix string, i int) string { return prefix + string(rune('a'+i)) }

func TestPrepareRoomsForExamsInSlot(t *testing.T) {
	dbClient := mongotest.NewDB(t)
	ctx := context.Background()
	p := &Plexams{dbClient: dbClient, semesterConfig: &model.SemesterConfig{}}

	// Rooms master data (plain rooms, no EXaHM/SEB/Lab), sorted large-to-small.
	rooms := []*model.Room{
		{Name: "R1", Seats: 30},
		{Name: "R2", Seats: 20},
		{Name: "R3", Seats: 10},
		{Name: "R4", Seats: 5},
	}
	roomInfo := make(map[string]*model.Room, len(rooms))
	roomNames := make([]string, 0, len(rooms))
	for _, r := range rooms {
		if _, err := p.dbClient.AddRoom(ctx, r); err != nil {
			t.Fatalf("AddRoom(%s): %v", r.Name, err)
		}
		roomInfo[r.Name] = r
		roomNames = append(roomNames, r.Name)
	}

	// Exam 100: 25 normal students, no NTA.
	exam100 := &model.AssembledExam{
		Ancode:           100,
		ZpaExam:          &model.ZPAExam{AnCode: 100, Module: "M100", MainExamer: "Prof A", Duration: 90},
		PrimussExams:     []*model.EnhancedPrimussExam{{StudentRegs: studentRegs("x", 25)}},
		StudentRegsCount: 25,
	}
	// Exam 200: 7 registrations = 5 normal + 1 NTA in a normal room (+50%) + 1 NTA needing
	// its own room (+25%). The two NTA Mtknrs (yf, yg) are excluded from the normal regs.
	ntaNormalMtknr, ntaAloneMtknr := mtknr("y", 5), mtknr("y", 6)
	exam200 := &model.AssembledExam{
		Ancode:       200,
		ZpaExam:      &model.ZPAExam{AnCode: 200, Module: "M200", MainExamer: "Prof B", Duration: 60},
		PrimussExams: []*model.EnhancedPrimussExam{{StudentRegs: studentRegs("y", 7)}},
		Ntas: []*model.NTA{
			{Mtknr: ntaNormalMtknr, Name: "NTA Normal", DeltaDurationPercent: 50, NeedsRoomAlone: false},
			{Mtknr: ntaAloneMtknr, Name: "NTA Alone", DeltaDurationPercent: 25, NeedsRoomAlone: true},
		},
		StudentRegsCount: 7,
	}
	if err := p.dbClient.CacheAssembledExams(ctx, []*model.AssembledExam{exam100, exam200}); err != nil {
		t.Fatalf("CacheAssembledExams: %v", err)
	}
	for _, e := range []*model.AssembledExam{exam100, exam200} {
		if _, err := p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{DayNumber: 1, SlotNumber: 1, Ancode: e.Ancode}); err != nil {
			t.Fatalf("AddExamToSlot(%d): %v", e.Ancode, err)
		}
	}

	cfg := &prepareRoomsCfg{
		roomInfo:             roomInfo,
		prePlannedRooms:      map[int][]*model.PrePlannedRoom{},
		additionalSeats:      map[int]int{},
		slot:                 &model.Slot{DayNumber: 1, SlotNumber: 1},
		roomsNotUsableInSlot: set.NewSet[string](),
		blockedRooms:         map[SlotNumber]set.Set[string]{},
		exactSeatRooms:       map[int]map[string]bool{},
		roomsForSlots:        map[SlotNumber][]string{{day: 1, slot: 1}: roomNames},
	}

	plannedRooms, unplaced, err := p.prepareRoomsForExamsInSlot(ctx, cfg, newDiscardReporter())
	if err != nil {
		t.Fatalf("prepareRoomsForExamsInSlot: %v", err)
	}

	if len(unplaced) != 0 {
		t.Errorf("unplaced = %+v, want none (enough rooms)", unplaced)
	}

	// Every seat is accounted for: 25 (exam100) + 5 normal + 1 NTA-normal + 1 NTA-alone = 32.
	totalStudents := 0
	ntaAloneRooms, ntaNormalRooms := 0, 0
	perAncode := map[int]int{}
	for _, r := range plannedRooms {
		if r.Reserve {
			continue
		}
		totalStudents += len(r.StudentsInRoom)
		perAncode[r.Ancode] += len(r.StudentsInRoom)
		switch {
		case r.HandicapRoomAlone:
			ntaAloneRooms++
			if len(r.StudentsInRoom) != 1 || r.NtaMtknr == nil || *r.NtaMtknr != ntaAloneMtknr {
				t.Errorf("NTA-alone room wrong: %+v", r)
			}
		case r.Handicap:
			ntaNormalRooms++
			if r.NtaMtknr == nil || *r.NtaMtknr != ntaNormalMtknr {
				t.Errorf("NTA-normal room wrong: %+v", r)
			}
		}
	}
	if totalStudents != 32 {
		t.Errorf("total students placed = %d, want 32", totalStudents)
	}
	if perAncode[100] != 25 {
		t.Errorf("exam 100 placed %d students, want 25", perAncode[100])
	}
	if perAncode[200] != 7 {
		t.Errorf("exam 200 placed %d students (incl. NTAs), want 7", perAncode[200])
	}
	if ntaAloneRooms != 1 {
		t.Errorf("NTA-alone rooms = %d, want 1", ntaAloneRooms)
	}
	if ntaNormalRooms != 1 {
		t.Errorf("NTA-normal (handicap, not alone) rooms = %d, want 1", ntaNormalRooms)
	}
}
