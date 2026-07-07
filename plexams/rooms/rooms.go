// Package rooms holds the stateful per-slot room-allocation machine that assigns
// concrete rooms to the exams planned in a slot. It was extracted verbatim from
// the plexams package (roomsPrepare.go); the logic is unchanged.
//
// The machine is driven by a *Cfg that carries both the static configuration
// (room master data, pre-planned rooms, blocked rooms, the allowed rooms per
// slot, …) and the per-slot mutable state. The plexams package builds the Cfg
// (populating the exported fields) and drives the outer slot loop, calling
// PrepareForSlot once per slot; the roomsNotUsableInSlot carryover between slots
// therefore lives in the outer loop / the Cfg, not here.
//
// It depends only on a small DB interface (ExamsInSlot + RoomByName, satisfied
// by *db.DB), a minimal Reporter (Step/Println/Warnf, satisfied structurally by
// the plexams reporter), and the pure roomcalc math.
package rooms

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/roomcalc"
	"github.com/rs/zerolog/log"
)

// SlotKey identifies a slot by day and slot number. It is the rooms-package-local
// counterpart of plexams.SlotNumber (which stays in plexams because it is used
// package-wide); the plexams Cfg builder converts its SlotNumber-keyed maps to
// SlotKey at the boundary.
type SlotKey struct {
	Day, Slot int
}

// slotStartPtr returns a fresh pointer to the current slot's absolute start time,
// used as the persisted coordinate of planned/unplaced rooms.
func slotStartPtr(cfg *Cfg) *time.Time {
	s := cfg.Slot.Starttime
	return &s
}

// DB is the persistence the room machine needs; *db.DB satisfies it.
type DB interface {
	ExamsAt(ctx context.Context, starttime time.Time) ([]*model.PlannedExam, error)
	RoomByName(ctx context.Context, roomName string) (*model.Room, error)
}

// Reporter is the subset of the plexams progress reporter this machine uses.
type Reporter interface {
	Step(msg string)
	Println(a ...any)
	Warnf(format string, a ...any)
}

// Cfg carries the static configuration and the per-slot mutable state of the
// room-allocation machine. The exported fields are populated by the plexams
// builder (and the characterization test); the unexported fields are the runtime
// state set during PrepareForSlot.
type Cfg struct {
	RoomInfo             map[string]*model.Room
	PrePlannedRooms      map[int][]*model.PrePlannedRoom
	AdditionalSeats      map[int]int
	Slot                 *model.Slot
	RoomsNotUsableInSlot set.Set[string]
	BlockedRooms         map[SlotKey]set.Set[string] // slot -> blocked room names
	ExactSeatRooms       map[int]map[string]bool     // ancode -> room names with an exact seat count (do not refill with this exam)
	RoomsForSlots        map[SlotKey][]string        // slot -> allowed room names (computed once)
	// SlotBlockMinutes is the spacing between consecutive slot start times (e.g. 120);
	// RoomTurnaroundMinutes is the ordinary Vorlauf/Nachlauf (15). Together they decide
	// whether an exam with an extended Nachlauf keeps its room past the following slot's
	// start (see roomOccupancyOverrunsSlot). 0 disables the check (default room turnaround
	// stays governed by the NTA-overrun rule only).
	SlotBlockMinutes      int
	RoomTurnaroundMinutes int

	exams                     []*model.ExamWithRegsAndRooms
	examsMap                  map[int]*model.PlannedExam
	availableRooms            []*model.Room
	plannedRoomsWithFreeSeats map[string]*plannedRoomsWithFreeSeats // key is room name
}

type plannedRoomsWithFreeSeats struct {
	plannedRooms []*model.PlannedRoom // one room can have multiple planned rooms
	freeSeats    int
}

// PrepareForSlot assigns rooms to all exams planned in cfg.Slot and returns the
// planned rooms plus any students that could not be placed. It mutates cfg's
// runtime state (exams/availableRooms/plannedRoomsWithFreeSeats/RoomsNotUsableInSlot).
func PrepareForSlot(ctx context.Context, db DB, cfg *Cfg, reporter Reporter) ([]*model.PlannedRoom, []*model.UnplacedExam, error) {
	unplaced := make([]*model.UnplacedExam, 0)
	reporter.Step(aurora.Sprintf(aurora.Black("preparing data for slot (%d/%d)"),
		aurora.Yellow(cfg.Slot.DayNumber),
		aurora.Yellow(cfg.Slot.SlotNumber),
	))

	log.Debug().Int("day", cfg.Slot.DayNumber).Int("slot", cfg.Slot.SlotNumber).Msg("preparing rooms for slot")

	slotStart := cfg.Slot.Starttime
	examsInSlot, err := db.ExamsAt(ctx, slotStart)
	if err != nil {
		log.Error().Err(err).Int("day", cfg.Slot.DayNumber).Int("time", cfg.Slot.SlotNumber).
			Msg("error while trying to find exams in slot")
		return nil, nil, err
	}

	if len(examsInSlot) == 0 {
		cfg.RoomsNotUsableInSlot = set.NewSet[string]()
		return nil, nil, nil
	}

	reporter.Println(aurora.Sprintf(aurora.Blue("slot (%d/%d): start planning"),
		cfg.Slot.DayNumber, cfg.Slot.SlotNumber))

	if cfg.Slot.SlotNumber == 1 { // a new day
		cfg.RoomsNotUsableInSlot = set.NewSet[string]()
	}

	cfg.exams, cfg.examsMap = mkExamsMap(cfg, examsInSlot, reporter)
	cfg.availableRooms, err = availableRoomsInSlot(ctx, db, cfg)

	if err != nil {
		log.Error().Err(err).Int("day", cfg.Slot.DayNumber).Int("time", cfg.Slot.SlotNumber).
			Msg("error while trying to get rooms for slot")
		return nil, nil, err
	}

	cfg.plannedRoomsWithFreeSeats = make(map[string]*plannedRoomsWithFreeSeats)

	examRooms := setPrePlannedRooms(cfg, reporter)

	// rooms for students without NTA
	for len(cfg.exams) != 0 {
		sort.Slice(cfg.exams, func(i, j int) bool {
			return len(cfg.exams[i].NormalRegsMtknr)+len(cfg.exams[i].NtasInNormalRooms) >
				len(cfg.exams[j].NormalRegsMtknr)+len(cfg.exams[j].NtasInNormalRooms)
		})

		if len(cfg.exams[0].NormalRegsMtknr) == 0 {
			break
		}

		exam := cfg.exams[0]
		cfg.exams = cfg.exams[1:]

		reporter.Step(aurora.Sprintf(aurora.Cyan(" ↪ %d. %s (%s): %d of %d studs left"),
			exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
			len(exam.NormalRegsMtknr), exam.Exam.StudentRegsCount))

		var room *model.Room
		neededSeats := len(exam.NormalRegsMtknr) + len(exam.NtasInNormalRooms)
		if neededSeats < 10 || (exam.Exam.Constraints != nil && exam.Exam.Constraints.RoomConstraints != nil &&
			(exam.Exam.Constraints.RoomConstraints.Seb || exam.Exam.Constraints.RoomConstraints.Exahm)) { // if we need less than 10 seats, we can use a room with free seats
			room = findRoomWithFreeSeats(cfg, exam, neededSeats)
		}
		if room == nil {
			room = findRoom(cfg, exam)
			if room == nil {
				// No real room left for this exam's remaining normal students:
				// record them as unplaced (instead of a "No Room" placeholder)
				// and re-queue the exam with no normal regs so its NTAs are still
				// handled in the following passes.
				log.Error().Int("ancode", exam.Exam.Ancode).Int("students", len(exam.NormalRegsMtknr)).
					Msg("no room found for exam, students unplaced")
				unplaced = append(unplaced, &model.UnplacedExam{
					Ancode:    exam.Exam.Ancode,
					Starttime: slotStartPtr(cfg),
					Mtknrs:    append([]string{}, exam.NormalRegsMtknr...),
				})
				reporter.Println(aurora.Sprintf(aurora.Red("no room for %d student(s) of exam %d — unplaced"),
					len(exam.NormalRegsMtknr), exam.Exam.Ancode))
				exam.NormalRegsMtknr = nil
				cfg.exams = append(cfg.exams, exam)
				continue
			}
		}

		reserveRoom := false
		studentCountInRoom := room.Seats
		if studentCountInRoom > len(exam.NormalRegsMtknr) {
			studentCountInRoom = len(exam.NormalRegsMtknr)
			if len(exam.Rooms) > 0 && studentCountInRoom < 10 {
				reserveRoom = true
			}
		}

		studentsInRoom := exam.NormalRegsMtknr[:studentCountInRoom]
		exam.NormalRegsMtknr = exam.NormalRegsMtknr[studentCountInRoom:]

		examRoom := &model.PlannedRoom{
			Starttime:         slotStartPtr(cfg),
			RoomName:          room.Name,
			Ancode:            exam.Exam.Ancode,
			Duration:          exam.Exam.ZpaExam.Duration,
			Handicap:          false,
			HandicapRoomAlone: false,
			Reserve:           reserveRoom,
			StudentsInRoom:    studentsInRoom,
			NtaMtknr:          nil,
		}

		addPlannedRoom(cfg, exam, room, examRoom)
		cfg.exams = append(cfg.exams, exam)
		examRooms = append(examRooms, examRoom)

		reporter.Println(aurora.Sprintf(aurora.Green("added %s for %d students (max. %d)"),
			examRoom.RoomName, len(examRoom.StudentsInRoom), room.Seats))
	}

	// rooms for NTAs in normal rooms
	for _, exam := range cfg.exams {
		if len(exam.NtasInNormalRooms) == 0 {
			continue
		}

		reporter.Step(aurora.Sprintf(aurora.Magenta(" ↪ %d. %s (%s): %d students with NTA in normal rooms"),
			exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
			len(exam.NtasInNormalRooms)))

		maxDuration := exam.Exam.ZpaExam.Duration
		for _, nta := range exam.NtasInNormalRooms {
			ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
			if maxDuration < ntaDuration {
				maxDuration = ntaDuration
			}
		}
		var room *model.Room
		for _, plannedRoom := range exam.Rooms {
			if (maxDuration > 100 && (plannedRoom.RoomName == "R1.046" || plannedRoom.RoomName == "R1.049")) || plannedRoom.HandicapRoomAlone {
				continue
			}

			if cfg.RoomInfo[plannedRoom.RoomName].Seats >= len(plannedRoom.StudentsInRoom)+len(exam.NtasInNormalRooms) {
				room = cfg.RoomInfo[plannedRoom.RoomName]
				break
			}
		}
		if room == nil {
			room = findRoom(cfg, exam)
			if room == nil {
				// no real room for the NTAs in normal rooms: record them unplaced.
				log.Error().Int("ancode", exam.Exam.Ancode).Int("ntas", len(exam.NtasInNormalRooms)).
					Msg("no room found for NTAs in normal rooms, students unplaced")
				for _, nta := range exam.NtasInNormalRooms {
					mtknr := nta.Mtknr
					unplaced = append(unplaced, &model.UnplacedExam{
						Ancode:    exam.Exam.Ancode,
						Starttime: slotStartPtr(cfg),
						Mtknrs:    []string{mtknr},
						NtaMtknr:  &mtknr,
					})
				}
				reporter.Println(aurora.Sprintf(aurora.Red("no room for %d NTA(s) of exam %d — unplaced"),
					len(exam.NtasInNormalRooms), exam.Exam.Ancode))
				continue
			}
		}
		for _, nta := range exam.NtasInNormalRooms {
			ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
			examRoom := &model.PlannedRoom{
				Starttime:         slotStartPtr(cfg),
				RoomName:          room.Name,
				Ancode:            exam.Exam.Ancode,
				Duration:          ntaDuration,
				Handicap:          true,
				HandicapRoomAlone: false,
				Reserve:           false,
				StudentsInRoom:    []string{nta.Mtknr},
				NtaMtknr:          &nta.Mtknr,
			}
			addPlannedRoom(cfg, exam, room, examRoom)
			examRooms = append(examRooms, examRoom)
		}
		comment := ""
		if maxDuration > 100 {
			cfg.RoomsNotUsableInSlot.Add(room.Name)
			comment = aurora.Sprintf(aurora.Red(" ---  room %s not usable in next slot!"), aurora.Green(room.Name))
		}
		reporter.Println(aurora.Sprintf(aurora.Green("added %s for %d students with NTA (%d minuntes)%s"),
			room.Name, len(exam.NtasInNormalRooms), maxDuration, comment))
	}

	// rooms for NTAs in alone rooms
	for _, exam := range cfg.exams {
		if len(exam.NtasInAloneRooms) == 0 {
			continue
		}

		for _, nta := range exam.NtasInAloneRooms {
			reporter.Step(aurora.Sprintf(aurora.Magenta(" ↪ %d. %s (%s): planning room for %s (%s)."),
				exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
				nta.Name, nta.Mtknr))

			ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))

			room := findSmallestRoom(cfg, exam)
			if room == nil {
				// no real room for this NTA's own room: record it unplaced.
				log.Error().Int("ancode", exam.Exam.Ancode).Str("mtknr", nta.Mtknr).
					Msg("no room found for NTA alone room, student unplaced")
				mtknr := nta.Mtknr
				unplaced = append(unplaced, &model.UnplacedExam{
					Ancode:    exam.Exam.Ancode,
					Starttime: slotStartPtr(cfg),
					Mtknrs:    []string{mtknr},
					NtaMtknr:  &mtknr,
				})
				reporter.Println(aurora.Sprintf(aurora.Red("no room for NTA %s of exam %d — unplaced"),
					nta.Mtknr, exam.Exam.Ancode))
				continue
			}

			examRoom := &model.PlannedRoom{
				Starttime:         slotStartPtr(cfg),
				RoomName:          room.Name,
				Ancode:            exam.Exam.Ancode,
				Duration:          ntaDuration,
				Handicap:          true,
				HandicapRoomAlone: true,
				Reserve:           false,
				StudentsInRoom:    []string{nta.Mtknr},
				NtaMtknr:          &nta.Mtknr,
			}
			addPlannedRoom(cfg, exam, room, examRoom)
			examRooms = append(examRooms, examRoom)

			comment := ""
			if ntaDuration > 100 {
				cfg.RoomsNotUsableInSlot.Add(room.Name)
				comment = aurora.Sprintf(aurora.Red(" ---  room %s not usable in next slot!"), aurora.Green(room.Name))
			}
			reporter.Println(aurora.Sprintf(aurora.Green("added %s (%d minuntes)%s"),
				room.Name, ntaDuration, comment))
		}
	}

	examRooms = addReserveBuffer(cfg, examRooms, reporter)

	return examRooms, unplaced, nil
}

// takeReserveRoom removes and returns the smallest available room that satisfies
// the exam's constraints and has at least minSeats seats, or nil if none fits.
func takeReserveRoom(cfg *Cfg, constraints *model.Constraints, minSeats int) *model.Room {
	bestIdx := -1
	for i, room := range cfg.availableRooms {
		// handicap rooms are reserved for NTAs, never used as a (non-NTA) reserve.
		if room.Handicap || room.Seats < minSeats || !roomcalc.SatisfiesConstraints(room, constraints) {
			continue
		}
		if bestIdx == -1 || room.Seats < cfg.availableRooms[bestIdx].Seats {
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return nil
	}
	room := cfg.availableRooms[bestIdx]
	cfg.availableRooms = append(cfg.availableRooms[:bestIdx], cfg.availableRooms[bestIdx+1:]...)
	return room
}

// addReserveBuffer makes sure no exam is packed exactly full: if an exam has
// fewer free seats than its buffer, an additional free room is added as a reserve
// (if one is still available in the slot). Otherwise a warning is emitted; the
// rooms validation flags it as well.
func addReserveBuffer(cfg *Cfg, examRooms []*model.PlannedRoom, reporter Reporter) []*model.PlannedRoom {
	ancodes := make([]int, 0)
	seen := make(map[int]bool)
	for _, r := range examRooms {
		if !seen[r.Ancode] {
			seen[r.Ancode] = true
			ancodes = append(ancodes, r.Ancode)
		}
	}

	for _, ancode := range ancodes {
		free, normal := roomcalc.ExamFreeSeats(cfg.RoomInfo, examRooms, ancode)
		if normal == 0 {
			continue
		}
		buffer := roomcalc.FreeSeatsBuffer(normal)
		if free >= buffer {
			continue
		}
		exam := cfg.examsMap[ancode]
		if exam == nil {
			continue
		}
		needed := buffer - free
		if needed < 1 {
			needed = 1
		}
		room := takeReserveRoom(cfg, exam.Constraints, needed)
		if room == nil {
			room = takeReserveRoom(cfg, exam.Constraints, 1) // any room is better than none
		}
		if room == nil {
			reporter.Warnf(aurora.Sprintf(aurora.Red("exam %d: only %d free seat(s) for %d students, no free room left for a reserve"),
				ancode, free, normal))
			continue
		}
		examRoom := &model.PlannedRoom{
			Starttime:      slotStartPtr(cfg),
			RoomName:       room.Name,
			Ancode:         ancode,
			Duration:       exam.ZpaExam.Duration,
			Reserve:        true,
			StudentsInRoom: []string{},
		}
		examRooms = append(examRooms, examRoom)
		reporter.Println(aurora.Sprintf(aurora.Green("added reserve room %s for exam %d (only %d free of needed %d)"),
			room.Name, ancode, free, buffer))
	}
	return examRooms
}

func mkExamsMap(cfg *Cfg, examsInPlan []*model.PlannedExam, reporter Reporter) ([]*model.ExamWithRegsAndRooms, map[int]*model.PlannedExam) {
	exams := make([]*model.ExamWithRegsAndRooms, 0, len(examsInPlan))
	examsMap := make(map[int]*model.PlannedExam)
	for _, examInPlan := range examsInPlan {

		if examInPlan.Constraints != nil && examInPlan.Constraints.NotPlannedByMe {
			continue
		}

		normalRegs, ntasInNormalRooms, ntasInAloneRooms := roomcalc.ExamRegsAndNTAs(examInPlan)

		addSeats, ok := cfg.AdditionalSeats[examInPlan.Ancode]
		if ok {
			reporter.Println(aurora.Sprintf(aurora.BrightRed("   adding %d seats to %d. %s (%s)"),
				addSeats, examInPlan.Ancode, examInPlan.ZpaExam.Module, examInPlan.ZpaExam.MainExamer))
			for i := 0; i < addSeats; i++ {
				normalRegs = append(normalRegs, "dummy")
			}
		}

		exams = append(exams, &model.ExamWithRegsAndRooms{
			Exam:              examInPlan,
			NormalRegsMtknr:   normalRegs,
			NtasInNormalRooms: ntasInNormalRooms,
			NtasInAloneRooms:  ntasInAloneRooms,
			Rooms:             make([]*model.PlannedRoom, 0),
		})
		examsMap[examInPlan.Ancode] = examInPlan
	}
	return exams, examsMap
}

func availableRoomsInSlot(ctx context.Context, db DB, cfg *Cfg) ([]*model.Room, error) {
	slotRoomNames := cfg.RoomsForSlots[SlotKey{
		Day:  cfg.Slot.DayNumber,
		Slot: cfg.Slot.SlotNumber,
	}]

	roomNames := set.NewSet(slotRoomNames...).Difference(cfg.RoomsNotUsableInSlot)
	cfg.RoomsNotUsableInSlot = set.NewSet[string]()

	rooms := make([]*model.Room, 0, roomNames.Cardinality())
	for roomName := range roomNames.Iter() {
		room, err := db.RoomByName(ctx, roomName)
		if err != nil {
			log.Error().Err(err).Str("roomName", roomName).Msg("error while trying to get room by name")
			continue
		}
		rooms = append(rooms, room)
	}
	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].Seats > rooms[j].Seats
	})

	return rooms, nil
}

func setPrePlannedRooms(cfg *Cfg, reporter Reporter) []*model.PlannedRoom {
	examRooms := make([]*model.PlannedRoom, 0)

	// Seats taken per room, tracked across all exams in the slot. A room that is
	// pre-planned for more than one (concurrent) exam must not be overbooked, so
	// the count has to be shared between exams instead of being reset per exam.
	seatsTakenMap := make(map[string]int) // room.name -> seats taken

	// Determine which rooms are pre-planned for more than one exam in this slot.
	// These shared rooms are filled last (see below), so that every exam first
	// fills its own (exclusive) rooms and only the leftover students end up in
	// the shared room.
	roomExams := make(map[string]set.Set[int]) // roomName -> set of ancodes
	for _, exam := range cfg.exams {
		for _, prePlannedRoom := range cfg.PrePlannedRooms[exam.Exam.Ancode] {
			if _, ok := roomExams[prePlannedRoom.RoomName]; !ok {
				roomExams[prePlannedRoom.RoomName] = set.NewSet[int]()
			}
			roomExams[prePlannedRoom.RoomName].Add(exam.Exam.Ancode)
		}
	}
	sharedRooms := set.NewSet[string]()
	for roomName, ancodes := range roomExams {
		if ancodes.Cardinality() > 1 {
			sharedRooms.Add(roomName)
		}
	}

	// isShared reports whether a pre-planned room must be deferred to the last
	// pass. NTA rooms (Mtknr != nil) are always assigned in the first pass (the
	// NTA has a fixed seat); their seats are still counted in seatsTakenMap so the
	// shared pass sees the correct remaining capacity.
	isShared := func(prePlannedRoom *model.PrePlannedRoom) bool {
		return prePlannedRoom.Mtknr == nil && sharedRooms.Contains(prePlannedRoom.RoomName)
	}

	// Pass 1: exclusive pre-planned rooms (and all NTA rooms).
	for _, exam := range cfg.exams {
		prePlannedRooms, ok := cfg.PrePlannedRooms[exam.Exam.Ancode]
		if !ok {
			continue
		}
		exclusive := make([]*model.PrePlannedRoom, 0, len(prePlannedRooms))
		for _, prePlannedRoom := range prePlannedRooms {
			if !isShared(prePlannedRoom) {
				exclusive = append(exclusive, prePlannedRoom)
			}
		}
		examRooms = append(examRooms, assignPrePlannedRooms(cfg, exam, exclusive, seatsTakenMap, reporter)...)
	}

	if sharedRooms.Cardinality() == 0 {
		return examRooms
	}

	// Pass 2: shared pre-planned rooms last, larger exam (more remaining normal
	// regs) first, so that if the shared room is too small the bigger exam gets
	// the seats and the overflow of the smaller one falls through to the normal
	// room allocation.
	examsBySize := make([]*model.ExamWithRegsAndRooms, len(cfg.exams))
	copy(examsBySize, cfg.exams)
	sort.SliceStable(examsBySize, func(i, j int) bool {
		return len(examsBySize[i].NormalRegsMtknr) > len(examsBySize[j].NormalRegsMtknr)
	})

	for _, exam := range examsBySize {
		prePlannedRooms, ok := cfg.PrePlannedRooms[exam.Exam.Ancode]
		if !ok {
			continue
		}
		shared := make([]*model.PrePlannedRoom, 0)
		for _, prePlannedRoom := range prePlannedRooms {
			if isShared(prePlannedRoom) {
				shared = append(shared, prePlannedRoom)
			}
		}
		if len(shared) == 0 {
			continue
		}
		examRooms = append(examRooms, assignPrePlannedRooms(cfg, exam, shared, seatsTakenMap, reporter)...)
	}

	return examRooms
}

// assignPrePlannedRooms fills the given pre-planned rooms of a single exam,
// honoring the slot-wide seatsTakenMap so that rooms shared between concurrent
// exams are not overbooked.
func assignPrePlannedRooms(cfg *Cfg, exam *model.ExamWithRegsAndRooms,
	prePlannedRooms []*model.PrePlannedRoom, seatsTakenMap map[string]int, reporter Reporter) []*model.PlannedRoom {
	examRooms := make([]*model.PlannedRoom, 0, len(prePlannedRooms))
	if len(prePlannedRooms) == 0 {
		return examRooms
	}

	reporter.Step(aurora.Sprintf(aurora.Cyan("found preplanned rooms for %d. %s (%s)"),
		aurora.Magenta(exam.Exam.Ancode),
		aurora.Magenta(exam.Exam.ZpaExam.Module),
		aurora.Magenta(exam.Exam.ZpaExam.MainExamer)))

	for _, prePlannedRoom := range prePlannedRooms {
		room, ok := cfg.RoomInfo[prePlannedRoom.RoomName]
		if !ok {
			log.Error().Str("roomName", prePlannedRoom.RoomName).Msg("pre-planned room not found in room info")
			panic(fmt.Sprintf("pre-planned room %s not found in room info", prePlannedRoom.RoomName))
		}

		// a room blocked for this slot wins over the pre-planning: skip it.
		slotKey := SlotKey{Day: cfg.Slot.DayNumber, Slot: cfg.Slot.SlotNumber}
		if blocked, ok := cfg.BlockedRooms[slotKey]; ok && blocked.Contains(prePlannedRoom.RoomName) {
			reporter.Warnf(aurora.Sprintf(
				aurora.Red("pre-planned room %s for %d is blocked in slot (%d,%d); skipped"),
				prePlannedRoom.RoomName, exam.Exam.Ancode, slotKey.Day, slotKey.Slot))
			continue
		}

		var examRoom *model.PlannedRoom
		if prePlannedRoom.Mtknr == nil { // room for normal students
			seatsTaken := seatsTakenMap[room.Name]
			studentCountInRoom := room.Seats - seatsTaken
			if studentCountInRoom < 0 {
				studentCountInRoom = 0
			}
			if studentCountInRoom > len(exam.NormalRegsMtknr) {
				studentCountInRoom = len(exam.NormalRegsMtknr)
			}

			// An optional exact seat count from the pre-planning is honored: exactly
			// that many students go into the room (capped by the free seats and the
			// remaining students; a warning is emitted if it cannot be fully met).
			if prePlannedRoom.Seats != nil {
				if *prePlannedRoom.Seats > studentCountInRoom {
					reporter.Warnf(aurora.Sprintf(
						aurora.Red("pre-planned %d seats in %s for exam %d, but only %d possible (free seats / remaining students)"),
						*prePlannedRoom.Seats, room.Name, exam.Exam.Ancode, studentCountInRoom))
				} else {
					studentCountInRoom = *prePlannedRoom.Seats
				}
				// the remaining capacity stays free for OTHER exams, but this exam
				// must not later refill its own exact room.
				if cfg.ExactSeatRooms[exam.Exam.Ancode] == nil {
					cfg.ExactSeatRooms[exam.Exam.Ancode] = make(map[string]bool)
				}
				cfg.ExactSeatRooms[exam.Exam.Ancode][room.Name] = true
			}

			// A shared room may already be full (taken by another exam) when this
			// exam gets to it; don't create an empty room entry then (reserve rooms
			// are intentional placeholders and are kept).
			if studentCountInRoom == 0 && !prePlannedRoom.Reserve {
				continue
			}

			studentsInRoom := exam.NormalRegsMtknr[:studentCountInRoom]
			exam.NormalRegsMtknr = exam.NormalRegsMtknr[studentCountInRoom:]

			examRoom = &model.PlannedRoom{
				Starttime:      slotStartPtr(cfg),
				RoomName:       room.Name,
				Ancode:         exam.Exam.Ancode,
				Duration:       exam.Exam.ZpaExam.Duration,
				StudentsInRoom: studentsInRoom,
				Reserve:        prePlannedRoom.Reserve,
				PrePlanned:     true,
			}
			seatsTakenMap[room.Name] = seatsTaken + studentCountInRoom
		} else { // room for NTA
			// Use the NTA that is already attached to the exam. It is the same
			// (active) record the non-pre-planned path uses; re-fetching it via
			// p.dbClient.Nta() can return a deactivated record with an outdated
			// DeltaDurationPercent and thus a wrong duration.
			var nta *model.NTA
			for i, candidate := range exam.NtasInNormalRooms {
				if candidate.Mtknr == *prePlannedRoom.Mtknr {
					nta = candidate
					exam.NtasInNormalRooms = append(exam.NtasInNormalRooms[:i], exam.NtasInNormalRooms[i+1:]...)
					break
				}
			}
			if nta == nil {
				for i, candidate := range exam.NtasInAloneRooms {
					if candidate.Mtknr == *prePlannedRoom.Mtknr {
						nta = candidate
						exam.NtasInAloneRooms = append(exam.NtasInAloneRooms[:i], exam.NtasInAloneRooms[i+1:]...)
						break
					}
				}
			}
			if nta == nil {
				log.Error().Str("mtknr", *prePlannedRoom.Mtknr).Msg("NTA not found in exam")
				continue
			}
			duration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
			examRoom = &model.PlannedRoom{
				Starttime:         slotStartPtr(cfg),
				RoomName:          room.Name,
				Ancode:            exam.Exam.Ancode,
				Duration:          duration,
				StudentsInRoom:    []string{nta.Mtknr},
				Handicap:          true,
				HandicapRoomAlone: nta.NeedsRoomAlone,
				Reserve:           false,
				NtaMtknr:          prePlannedRoom.Mtknr,
				PrePlanned:        true,
			}
			seatsTakenMap[room.Name]++
		}
		addPlannedRoom(cfg, exam, room, examRoom)

		examRooms = append(examRooms, examRoom)
	}
	reporter.Println(aurora.Sprintf(aurora.Green("added %d room(s)"), len(prePlannedRooms)))

	return examRooms
}

func addPlannedRoom(cfg *Cfg, exam *model.ExamWithRegsAndRooms, room *model.Room, examRoom *model.PlannedRoom) {
	exam.Rooms = append(exam.Rooms, examRoom)
	if !examRoom.HandicapRoomAlone && room.Seats-len(examRoom.StudentsInRoom)-len(exam.NtasInNormalRooms) > 0 {
		if plannedRooms, ok := cfg.plannedRoomsWithFreeSeats[room.Name]; ok {
			plannedRooms.plannedRooms = append(plannedRooms.plannedRooms, examRoom)
			plannedRooms.freeSeats -= (len(examRoom.StudentsInRoom) + len(exam.NtasInNormalRooms))
		} else {
			cfg.plannedRoomsWithFreeSeats[room.Name] = &plannedRoomsWithFreeSeats{
				plannedRooms: []*model.PlannedRoom{examRoom},
				freeSeats:    room.Seats - (len(examRoom.StudentsInRoom) + len(exam.NtasInNormalRooms)),
			}
		}
	}
	// Remove the room from availableRooms
	for i, r := range cfg.availableRooms {
		if r.Name == room.Name {
			cfg.availableRooms = append(cfg.availableRooms[:i], cfg.availableRooms[i+1:]...)
			break
		}
	}
	// An exam with an extended Nachlauf keeps this room occupied past the ordinary
	// turnaround; if that reaches into the following slot, the room must not be reused
	// there (the time-based counterpart of the NTA-overrun rule above).
	if cfg.roomOccupancyOverrunsSlot(exam.Exam.Constraints, examRoom.Duration) {
		cfg.RoomsNotUsableInSlot.Add(room.Name)
	}
}

// roomOccupancyOverrunsSlot reports whether an exam that asks for an extended Nachlauf
// keeps its room past the following slot's start. Occupancy ends at start + examDuration +
// Nachlauf; the next slot's exam needs the room from nextStart − turnaround = start +
// SlotBlockMinutes − RoomTurnaroundMinutes. It fires only for a Nachlauf larger than the
// ordinary turnaround, so exams on the default buffer keep their existing behavior.
func (cfg *Cfg) roomOccupancyOverrunsSlot(constraints *model.Constraints, examDuration int) bool {
	if cfg.SlotBlockMinutes <= 0 {
		return false
	}
	post := cfg.RoomTurnaroundMinutes
	if constraints != nil && constraints.RoomConstraints != nil && constraints.RoomConstraints.PostExamMinutes != nil {
		if p := *constraints.RoomConstraints.PostExamMinutes; p > post {
			post = p
		}
	}
	if post <= cfg.RoomTurnaroundMinutes {
		return false
	}
	return examDuration+post > cfg.SlotBlockMinutes-cfg.RoomTurnaroundMinutes
}

func findRoomWithFreeSeats(cfg *Cfg, exam *model.ExamWithRegsAndRooms, neededSeats int) *model.Room {
	if len(cfg.plannedRoomsWithFreeSeats) == 0 {
		return nil
	}

OUTER:
	for roomName, plannedRoomWithFreeSeats := range cfg.plannedRoomsWithFreeSeats {
		// do not refill a room this exam already filled to an exact seat count.
		if cfg.ExactSeatRooms[exam.Exam.Ancode][roomName] {
			continue
		}
		if plannedRoomWithFreeSeats.freeSeats >= neededSeats {
			room := cfg.RoomInfo[roomName]
			if !room.Handicap && roomcalc.SatisfiesConstraints(room, exam.Exam.Constraints) {
				for _, plannedRoom := range plannedRoomWithFreeSeats.plannedRooms {
					otherExam := cfg.examsMap[plannedRoom.Ancode]
					if exam.Exam.ZpaExam.Duration != otherExam.ZpaExam.Duration {
						continue OUTER
					}
				}
				return room
			}
		}
	}

	return nil
}

func findRoom(cfg *Cfg, exam *model.ExamWithRegsAndRooms) *model.Room {
	for i, room := range cfg.availableRooms {
		// handicap rooms are reserved for NTAs (placed via findSmallestRoom).
		if !room.Handicap && roomcalc.SatisfiesConstraints(room, exam.Exam.Constraints) {
			// remove room from available rooms
			cfg.availableRooms = append(cfg.availableRooms[:i], cfg.availableRooms[i+1:]...)
			return room
		}
	}
	return nil
}

func findSmallestRoom(cfg *Cfg, exam *model.ExamWithRegsAndRooms) *model.Room {
	for i := len(cfg.availableRooms) - 1; i >= 0; i-- {
		room := cfg.availableRooms[i]
		if roomcalc.SatisfiesConstraints(room, exam.Exam.Constraints) {
			// remove room from available rooms
			cfg.availableRooms = append(cfg.availableRooms[:i], cfg.availableRooms[i+1:]...)
			return room
		}
	}
	return nil
}
