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

// roomsTestPlexams returns a *Plexams on a throwaway DB with the given rooms added, plus a
// roomInfo map and the room-name list (large-to-small order preserved).
func roomsTestPlexams(t *testing.T, rooms []*model.Room) (*Plexams, context.Context, map[string]*model.Room, []string) {
	t.Helper()
	dbClient := mongotest.NewDB(t)
	ctx := context.Background()
	p := &Plexams{dbClient: dbClient, semesterConfig: &model.SemesterConfig{}}
	roomInfo := make(map[string]*model.Room, len(rooms))
	roomNames := make([]string, 0, len(rooms))
	for _, r := range rooms {
		if _, err := p.dbClient.AddRoom(ctx, r); err != nil {
			t.Fatalf("AddRoom(%s): %v", r.Name, err)
		}
		roomInfo[r.Name] = r
		roomNames = append(roomNames, r.Name)
	}
	return p, ctx, roomInfo, roomNames
}

// simpleExam is one assembled exam with n normal students (no NTA) at the given duration.
func simpleExam(ancode, students, duration int) *model.AssembledExam {
	return &model.AssembledExam{
		Ancode:           ancode,
		ZpaExam:          &model.ZPAExam{AnCode: ancode, Module: "M", MainExamer: "Prof", Duration: duration},
		PrimussExams:     []*model.EnhancedPrimussExam{{StudentRegs: studentRegs("s", students)}},
		StudentRegsCount: students,
	}
}

// seedSlot caches the assembled exams and adds a plan entry for each in slot (1,1).
func seedSlot(t *testing.T, p *Plexams, ctx context.Context, exams ...*model.AssembledExam) {
	t.Helper()
	if err := p.dbClient.CacheAssembledExams(ctx, exams); err != nil {
		t.Fatalf("CacheAssembledExams: %v", err)
	}
	for _, e := range exams {
		if _, err := p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{DayNumber: 1, SlotNumber: 1, Ancode: e.Ancode}); err != nil {
			t.Fatalf("AddExamToSlot(%d): %v", e.Ancode, err)
		}
	}
}

// slotCfg builds a prepareRoomsCfg for slot (1,1) with the given rooms available.
func slotCfg(roomInfo map[string]*model.Room, roomNames []string) *prepareRoomsCfg {
	return &prepareRoomsCfg{
		roomInfo:             roomInfo,
		prePlannedRooms:      map[int][]*model.PrePlannedRoom{},
		additionalSeats:      map[int]int{},
		slot:                 &model.Slot{DayNumber: 1, SlotNumber: 1},
		roomsNotUsableInSlot: set.NewSet[string](),
		blockedRooms:         map[SlotNumber]set.Set[string]{},
		exactSeatRooms:       map[int]map[string]bool{},
		roomsForSlots:        map[SlotNumber][]string{{day: 1, slot: 1}: roomNames},
	}
}

// TestPrepareRoomsExahmConstraint pins the room-constraint filtering: an EXaHM exam is
// placed only in an EXaHM room, never in a plain room, even if a bigger plain room is free.
func TestPrepareRoomsExahmConstraint(t *testing.T) {
	// R1 is the biggest but plain; E1 is the only EXaHM room.
	p, ctx, roomInfo, roomNames := roomsTestPlexams(t, []*model.Room{{Name: "R1", Seats: 30}, {Name: "E1", Seats: 25, Exahm: true}})
	exam := simpleExam(400, 15, 90)
	exam.Constraints = &model.Constraints{RoomConstraints: &model.RoomConstraints{Exahm: true}}
	seedSlot(t, p, ctx, exam)

	plannedRooms, unplaced, err := p.prepareRoomsForExamsInSlot(ctx, slotCfg(roomInfo, roomNames), newDiscardReporter())
	if err != nil {
		t.Fatalf("prepareRoomsForExamsInSlot: %v", err)
	}
	if len(unplaced) != 0 {
		t.Errorf("unplaced = %+v, want none", unplaced)
	}
	placed := 0
	for _, r := range plannedRooms {
		if r.Reserve {
			continue
		}
		placed += len(r.StudentsInRoom)
		if r.RoomName != "E1" {
			t.Errorf("EXaHM exam placed in %s, want the EXaHM room E1 only", r.RoomName)
		}
	}
	if placed != 15 {
		t.Errorf("placed = %d, want 15", placed)
	}
}

// TestPrepareRoomsPrePlanned pins the pre-planned-room path: a room pre-planned for an exam
// is filled first (marked PrePlanned), and the students it holds are not re-placed elsewhere.
func TestPrepareRoomsPrePlanned(t *testing.T) {
	p, ctx, roomInfo, roomNames := roomsTestPlexams(t, []*model.Room{{Name: "R1", Seats: 30}, {Name: "R2", Seats: 25}})
	seedSlot(t, p, ctx, simpleExam(300, 20, 90)) // 20 students, pre-planned into R2

	cfg := slotCfg(roomInfo, roomNames)
	cfg.prePlannedRooms = map[int][]*model.PrePlannedRoom{
		300: {{Ancode: 300, RoomName: "R2"}},
	}

	plannedRooms, unplaced, err := p.prepareRoomsForExamsInSlot(ctx, cfg, newDiscardReporter())
	if err != nil {
		t.Fatalf("prepareRoomsForExamsInSlot: %v", err)
	}
	if len(unplaced) != 0 {
		t.Errorf("unplaced = %+v, want none", unplaced)
	}
	if len(plannedRooms) != 1 {
		t.Fatalf("planned rooms = %d, want 1 (all 20 fit the pre-planned R2)", len(plannedRooms))
	}
	r := plannedRooms[0]
	if r.RoomName != "R2" || !r.PrePlanned || len(r.StudentsInRoom) != 20 {
		t.Errorf("pre-planned room = %+v, want R2 PrePlanned with 20 students", r)
	}
}

// TestPrepareRoomsUnplacedWhenRoomsExhausted pins the overflow path: when the available
// rooms cannot seat all normal students, the remainder is reported as unplaced (not lost).
func TestPrepareRoomsUnplacedWhenRoomsExhausted(t *testing.T) {
	p, ctx, roomInfo, roomNames := roomsTestPlexams(t, []*model.Room{{Name: "R1", Seats: 30}})
	seedSlot(t, p, ctx, simpleExam(100, 40, 90)) // 40 students, only 30 seats available

	plannedRooms, unplaced, err := p.prepareRoomsForExamsInSlot(ctx, slotCfg(roomInfo, roomNames), newDiscardReporter())
	if err != nil {
		t.Fatalf("prepareRoomsForExamsInSlot: %v", err)
	}

	placed := 0
	for _, r := range plannedRooms {
		if !r.Reserve {
			placed += len(r.StudentsInRoom)
		}
	}
	unplacedCount := 0
	for _, u := range unplaced {
		unplacedCount += len(u.Mtknrs)
	}
	if placed != 30 {
		t.Errorf("placed = %d, want 30 (the single room)", placed)
	}
	if unplacedCount != 10 {
		t.Errorf("unplaced = %d, want 10 (40 - 30)", unplacedCount)
	}
}

// TestPrepareRoomsReserveBuffer pins the addReserveBuffer path: an exam packed to fewer
// than its free-seat buffer gets an extra reserve room (if one is still available).
func TestPrepareRoomsReserveBuffer(t *testing.T) {
	// R1 has exactly 29 seats: 29 students -> 0 free, below the buffer of max(2, 5%) -> a
	// reserve room is taken from the remaining rooms.
	p, ctx, roomInfo, roomNames := roomsTestPlexams(t, []*model.Room{{Name: "R1", Seats: 29}, {Name: "R2", Seats: 20}})
	seedSlot(t, p, ctx, simpleExam(100, 29, 90))

	plannedRooms, unplaced, err := p.prepareRoomsForExamsInSlot(ctx, slotCfg(roomInfo, roomNames), newDiscardReporter())
	if err != nil {
		t.Fatalf("prepareRoomsForExamsInSlot: %v", err)
	}
	if len(unplaced) != 0 {
		t.Errorf("unplaced = %+v, want none", unplaced)
	}

	reserve, normal := 0, 0
	for _, r := range plannedRooms {
		if r.Reserve {
			reserve++
		} else {
			normal += len(r.StudentsInRoom)
		}
	}
	if normal != 29 {
		t.Errorf("normal seats placed = %d, want 29", normal)
	}
	if reserve != 1 {
		t.Errorf("reserve rooms = %d, want 1 (buffer not met -> one reserve added)", reserve)
	}
}

func TestPrepareRoomsForExamsInSlot(t *testing.T) {
	// Rooms master data (plain rooms, no EXaHM/SEB/Lab), sorted large-to-small.
	p, ctx, roomInfo, roomNames := roomsTestPlexams(t, []*model.Room{
		{Name: "R1", Seats: 30},
		{Name: "R2", Seats: 20},
		{Name: "R3", Seats: 10},
		{Name: "R4", Seats: 5},
	})

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
	seedSlot(t, p, ctx, exam100, exam200)

	plannedRooms, unplaced, err := p.prepareRoomsForExamsInSlot(ctx, slotCfg(roomInfo, roomNames), newDiscardReporter())
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
