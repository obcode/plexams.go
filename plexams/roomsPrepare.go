package plexams

import (
	"context"
	"fmt"
	"math"
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// PrepareRoomForExams assigns rooms to all planned exams and stores the result in
// planned_rooms. It first (re)computes the allowed rooms per slot
// (PrepareRoomsForSlots), so that step no longer has to be run separately: the
// room-for-exams generation always works on an up-to-date rooms-for-slots cache.
func (p *Plexams) PrepareRoomForExams(ctx context.Context, reporter Reporter) error {
	if err := p.generationAllowed(ctx, model.PlanningGateRooms); err != nil {
		return err
	}
	reporter.Println(aurora.Sprintf(aurora.Cyan("preparing rooms for slots")))
	if err := p.PrepareRoomsForSlots(ctx, reporter); err != nil {
		log.Error().Err(err).Msg("cannot prepare rooms for slots")
		return err
	}

	prepareRoomsCfg, err := p.prepareRoomsCfg(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get prepare rooms config")
		return err
	}

	reporter.Println(aurora.Sprintf(aurora.Cyan("preparing rooms for exams")))
	examRooms := make([]*model.PlannedRoom, 0)
	for _, slot := range p.semesterConfig.Slots {
		prepareRoomsCfg.slot = slot
		rooms, err := p.prepareRoomsForExamsInSlot(ctx, prepareRoomsCfg, reporter)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).
				Msg("error while preparing rooms for exams in slot")
			continue
		}
		examRooms = append(examRooms, rooms...)
	}

	if err := p.dbClient.ReplacePlannedRooms(ctx, examRooms); err != nil {
		return err
	}
	p.markCondition(ctx, condRoomsGenerated)
	reporter.StopProgress(fmt.Sprintf("%d planned rooms written", len(examRooms)))
	return nil
}

func (p *Plexams) prepareRoomsForExamsInSlot(ctx context.Context, prepareRoomsCfg *prepareRoomsCfg, reporter Reporter) ([]*model.PlannedRoom, error) {
	reporter.Step(aurora.Sprintf(aurora.Black("preparing data for slot (%d/%d)"),
		aurora.Yellow(prepareRoomsCfg.slot.DayNumber),
		aurora.Yellow(prepareRoomsCfg.slot.SlotNumber),
	))

	log.Debug().Int("day", prepareRoomsCfg.slot.DayNumber).Int("slot", prepareRoomsCfg.slot.SlotNumber).Msg("preparing rooms for slot")

	examsInSlot, err := p.ExamsInSlot(ctx, prepareRoomsCfg.slot.DayNumber, prepareRoomsCfg.slot.SlotNumber)
	if err != nil {
		log.Error().Err(err).Int("day", prepareRoomsCfg.slot.DayNumber).Int("time", prepareRoomsCfg.slot.SlotNumber).
			Msg("error while trying to find exams in slot")
		return nil, err
	}

	if len(examsInSlot) == 0 {
		prepareRoomsCfg.roomsNotUsableInSlot = set.NewSet[string]()
		return nil, nil
	}

	reporter.Println(aurora.Sprintf(aurora.Blue("slot (%d/%d): start planning"),
		prepareRoomsCfg.slot.DayNumber, prepareRoomsCfg.slot.SlotNumber))

	if prepareRoomsCfg.slot.SlotNumber == 1 { // a new day
		prepareRoomsCfg.roomsNotUsableInSlot = set.NewSet[string]()
	}

	prepareRoomsCfg.exams, prepareRoomsCfg.examsMap = p.mkExamsMap(prepareRoomsCfg, examsInSlot, reporter)
	prepareRoomsCfg.availableRooms, err = p.availableRoomsInSlot(ctx, prepareRoomsCfg)

	if err != nil {
		log.Error().Err(err).Int("day", prepareRoomsCfg.slot.DayNumber).Int("time", prepareRoomsCfg.slot.SlotNumber).
			Msg("error while trying to get rooms for slot")
		return nil, err
	}

	prepareRoomsCfg.plannedRoomsWithFreeSeats = make(map[string]*plannedRoomsWithFreeSeats)

	examRooms := p.setPrePlannedRooms(prepareRoomsCfg, reporter)

	// rooms for students without NTA
	for len(prepareRoomsCfg.exams) != 0 {
		sort.Slice(prepareRoomsCfg.exams, func(i, j int) bool {
			return len(prepareRoomsCfg.exams[i].NormalRegsMtknr)+len(prepareRoomsCfg.exams[i].NtasInNormalRooms) >
				len(prepareRoomsCfg.exams[j].NormalRegsMtknr)+len(prepareRoomsCfg.exams[j].NtasInNormalRooms)
		})

		if len(prepareRoomsCfg.exams[0].NormalRegsMtknr) == 0 {
			break
		}

		exam := prepareRoomsCfg.exams[0]
		prepareRoomsCfg.exams = prepareRoomsCfg.exams[1:]

		reporter.Step(aurora.Sprintf(aurora.Cyan(" ↪ %d. %s (%s): %d of %d studs left"),
			exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
			len(exam.NormalRegsMtknr), exam.Exam.StudentRegsCount))

		var room *model.Room
		neededSeats := len(exam.NormalRegsMtknr) + len(exam.NtasInNormalRooms)
		if neededSeats < 10 || (exam.Exam.Constraints != nil && exam.Exam.Constraints.RoomConstraints != nil &&
			(exam.Exam.Constraints.RoomConstraints.Seb || exam.Exam.Constraints.RoomConstraints.Exahm)) { // if we need less than 10 seats, we can use a room with free seats
			room = p.findRoomWithFreeSeats(prepareRoomsCfg, exam, neededSeats)
		}
		if room == nil {
			room = findRoom(prepareRoomsCfg, exam)
			if room == nil {
				log.Error().Int("ancode", exam.Exam.Ancode).Msg("no room found for exam")
				room = &model.Room{
					Name:  "No Room",
					Seats: 1000,
				}
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
			Day:               prepareRoomsCfg.slot.DayNumber,
			Slot:              prepareRoomsCfg.slot.SlotNumber,
			RoomName:          room.Name,
			Ancode:            exam.Exam.Ancode,
			Duration:          exam.Exam.ZpaExam.Duration,
			Handicap:          false,
			HandicapRoomAlone: false,
			Reserve:           reserveRoom,
			StudentsInRoom:    studentsInRoom,
			NtaMtknr:          nil,
		}

		p.addPlannedRoom(prepareRoomsCfg, exam, room, examRoom)
		prepareRoomsCfg.exams = append(prepareRoomsCfg.exams, exam)
		examRooms = append(examRooms, examRoom)

		reporter.Println(aurora.Sprintf(aurora.Green("added %s for %d students (max. %d)"),
			examRoom.RoomName, len(examRoom.StudentsInRoom), room.Seats))
	}

	// rooms for NTAs in normal rooms
	for _, exam := range prepareRoomsCfg.exams {
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

			if prepareRoomsCfg.roomInfo[plannedRoom.RoomName].Seats >= len(plannedRoom.StudentsInRoom)+len(exam.NtasInNormalRooms) {
				room = prepareRoomsCfg.roomInfo[plannedRoom.RoomName]
				break
			}
		}
		if room == nil {
			room = findRoom(prepareRoomsCfg, exam)
			if room == nil {
				log.Error().Int("ancode", exam.Exam.Ancode).Msg("no room found for exam")
				room = &model.Room{
					Name:  "No Room",
					Seats: 1000,
				}
			}
		}
		for _, nta := range exam.NtasInNormalRooms {
			ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
			examRoom := &model.PlannedRoom{
				Day:               prepareRoomsCfg.slot.DayNumber,
				Slot:              prepareRoomsCfg.slot.SlotNumber,
				RoomName:          room.Name,
				Ancode:            exam.Exam.Ancode,
				Duration:          ntaDuration,
				Handicap:          true,
				HandicapRoomAlone: false,
				Reserve:           false,
				StudentsInRoom:    []string{nta.Mtknr},
				NtaMtknr:          &nta.Mtknr,
			}
			p.addPlannedRoom(prepareRoomsCfg, exam, room, examRoom)
			examRooms = append(examRooms, examRoom)
		}
		comment := ""
		if maxDuration > 100 {
			prepareRoomsCfg.roomsNotUsableInSlot.Add(room.Name)
			comment = aurora.Sprintf(aurora.Red(" ---  room %s not usable in next slot!"), aurora.Green(room.Name))
		}
		reporter.Println(aurora.Sprintf(aurora.Green("added %s for %d students with NTA (%d minuntes)%s"),
			room.Name, len(exam.NtasInNormalRooms), maxDuration, comment))
	}

	// rooms for NTAs in alone rooms
	for _, exam := range prepareRoomsCfg.exams {
		if len(exam.NtasInAloneRooms) == 0 {
			continue
		}

		for _, nta := range exam.NtasInAloneRooms {
			reporter.Step(aurora.Sprintf(aurora.Magenta(" ↪ %d. %s (%s): planning room for %s (%s)."),
				exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
				nta.Name, nta.Mtknr))

			ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))

			room := findSmallestRoom(prepareRoomsCfg, exam)
			if room == nil {
				log.Error().Int("ancode", exam.Exam.Ancode).Msg("no room found for exam")
				room = &model.Room{
					Name:  "No Room",
					Seats: 1000,
				}
			}

			examRoom := &model.PlannedRoom{
				Day:               prepareRoomsCfg.slot.DayNumber,
				Slot:              prepareRoomsCfg.slot.SlotNumber,
				RoomName:          room.Name,
				Ancode:            exam.Exam.Ancode,
				Duration:          ntaDuration,
				Handicap:          true,
				HandicapRoomAlone: true,
				Reserve:           false,
				StudentsInRoom:    []string{nta.Mtknr},
				NtaMtknr:          &nta.Mtknr,
			}
			p.addPlannedRoom(prepareRoomsCfg, exam, room, examRoom)
			examRooms = append(examRooms, examRoom)

			comment := ""
			if ntaDuration > 100 {
				prepareRoomsCfg.roomsNotUsableInSlot.Add(room.Name)
				comment = aurora.Sprintf(aurora.Red(" ---  room %s not usable in next slot!"), aurora.Green(room.Name))
			}
			reporter.Println(aurora.Sprintf(aurora.Green("added %s (%d minuntes)%s"),
				room.Name, ntaDuration, comment))
		}
	}

	return examRooms, nil
}

type prepareRoomsCfg struct {
	roomInfo                  map[string]*model.Room
	prePlannedRooms           map[int][]*model.PrePlannedRoom
	additionalSeats           map[int]int
	slot                      *model.Slot
	exams                     []*model.ExamWithRegsAndRooms
	examsMap                  map[int]*model.PlannedExam
	availableRooms            []*model.Room
	plannedRoomsWithFreeSeats map[string]*plannedRoomsWithFreeSeats // key is room name
	roomsNotUsableInSlot      set.Set[string]
	blockedRooms              map[SlotNumber]set.Set[string] // slot -> blocked room names
}

type plannedRoomsWithFreeSeats struct {
	plannedRooms []*model.PlannedRoom // one room can have multiple planned rooms
	freeSeats    int
}

func (p *Plexams) prepareRoomsCfg(ctx context.Context) (*prepareRoomsCfg, error) {
	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return nil, err
	}

	roomInfo := make(map[string]*model.Room)
	for _, room := range allRooms {
		roomInfo[room.Name] = room
	}

	blocks, err := p.dbClient.BlockedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get blocked rooms")
		return nil, err
	}
	blockedRooms := make(map[SlotNumber]set.Set[string])
	for _, b := range blocks {
		key := SlotNumber{day: b.Day, slot: b.Slot}
		if _, ok := blockedRooms[key]; !ok {
			blockedRooms[key] = set.NewSet[string]()
		}
		blockedRooms[key].Add(b.Room)
	}

	prepareRoomsCfg := &prepareRoomsCfg{
		roomInfo:        roomInfo,
		prePlannedRooms: p.prePlannedRooms(ctx, roomInfo),
		additionalSeats: additionalSeats(),
		blockedRooms:    blockedRooms,
	}

	log.Info().Interface("prePlannedRooms", prepareRoomsCfg.prePlannedRooms).Msg("prepareRoomsCfg initialized")

	return prepareRoomsCfg, nil
}

func additionalSeats() map[int]int {
	additionalSeats := make(map[int]int) // ancode -> seats
	additionalSeatsViper := viper.Get("roomconstraints.additionalseats")

	additionalSeatsSlice, ok := additionalSeatsViper.([]interface{})
	if ok {
		for _, addSeat := range additionalSeatsSlice {
			entry, ok := addSeat.(map[string]interface{})
			if !ok {
				log.Error().Interface("addSeat", addSeat).Msg("cannot convert addSeat to map")
			}
			ancode, okAncode := entry["ancode"].(int)
			seats, okSeats := entry["seats"].(int)

			if okAncode && okSeats {
				additionalSeats[ancode] = seats
			}
		}
	}

	log.Debug().Interface("additionalSeats", additionalSeats).Msg("found additional seats")

	return additionalSeats
}

func (p *Plexams) prePlannedRooms(ctx context.Context, roomInfo map[string]*model.Room) map[int][]*model.PrePlannedRoom {
	prePlannedRoomsMap := make(map[int][]*model.PrePlannedRoom)
	prePlannedRooms, err := p.dbClient.PrePlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre-planned rooms")
		return nil
	}

	for _, room := range prePlannedRooms {
		if _, ok := prePlannedRoomsMap[room.Ancode]; !ok {
			prePlannedRoomsMap[room.Ancode] = make([]*model.PrePlannedRoom, 0, 1)
		}

		prePlannedRoomsMap[room.Ancode] = append(prePlannedRoomsMap[room.Ancode], room)
	}

	for _, rooms := range prePlannedRoomsMap {
		// Sort rooms within each exam's preplanned rooms
		// First: rooms with Mtknr != nil
		// Last: rooms with reserve == true
		// Middle: sort by seats (descending)
		sort.Slice(rooms, func(i, j int) bool {
			// First priority: rooms with Mtknr != nil come first
			if (rooms[i].Mtknr != nil) != (rooms[j].Mtknr != nil) {
				return rooms[i].Mtknr != nil
			}

			// If both have Mtknr != nil, keep original order
			if rooms[i].Mtknr != nil && rooms[j].Mtknr != nil {
				return false
			}

			// Last priority: rooms with reserve == true come last
			if rooms[i].Reserve != rooms[j].Reserve {
				return !rooms[i].Reserve
			}

			// If both are non-reserve rooms, sort by seats (descending)
			if !rooms[i].Reserve && !rooms[j].Reserve {
				seatsI := roomInfo[rooms[i].RoomName].Seats
				seatsJ := roomInfo[rooms[j].RoomName].Seats
				return seatsI > seatsJ
			}

			// Keep original order for reserve rooms
			return false
		})
	}

	return prePlannedRoomsMap
}

func (p *Plexams) mkExamsMap(prepareRoomsCfg *prepareRoomsCfg, examsInPlan []*model.PlannedExam, reporter Reporter) ([]*model.ExamWithRegsAndRooms, map[int]*model.PlannedExam) {
	exams := make([]*model.ExamWithRegsAndRooms, 0, len(examsInPlan))
	examsMap := make(map[int]*model.PlannedExam)
	for _, examInPlan := range examsInPlan {

		if examInPlan.Constraints != nil && examInPlan.Constraints.NotPlannedByMe {
			continue
		}

		ntas := examInPlan.Ntas
		ntaMtknrs := set.NewSet[string]()
		ntasInNormalRooms := make([]*model.NTA, 0)
		ntasInAloneRooms := make([]*model.NTA, 0)
		for _, nta := range ntas {
			ntaMtknrs.Add(nta.Mtknr)
			if nta.NeedsRoomAlone {
				ntasInAloneRooms = append(ntasInAloneRooms, nta)
			} else {
				ntasInNormalRooms = append(ntasInNormalRooms, nta) // nolint
			}
		}

		normalRegs := make([]string, 0)
		for _, primussExam := range examInPlan.PrimussExams {
			for _, studentRegs := range primussExam.StudentRegs {
				if !ntaMtknrs.Contains(studentRegs.Mtknr) {
					normalRegs = append(normalRegs, studentRegs.Mtknr)
				}
			}
		}

		addSeats, ok := prepareRoomsCfg.additionalSeats[examInPlan.Ancode]
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

func (p *Plexams) availableRoomsInSlot(ctx context.Context, prepareRoomsCfg *prepareRoomsCfg) ([]*model.Room, error) {
	slotWithRooms, err := p.RoomsForSlot(ctx, prepareRoomsCfg.slot.DayNumber, prepareRoomsCfg.slot.SlotNumber)
	if err != nil {
		log.Error().Err(err).Int("day", prepareRoomsCfg.slot.DayNumber).Int("time", prepareRoomsCfg.slot.SlotNumber).
			Msg("error while trying to get rooms for slot")
		return nil, err
	}

	roomNames := set.NewSet(slotWithRooms.RoomNames...).Difference(prepareRoomsCfg.roomsNotUsableInSlot)
	prepareRoomsCfg.roomsNotUsableInSlot = set.NewSet[string]()

	rooms := make([]*model.Room, 0, roomNames.Cardinality())
	for roomName := range roomNames.Iter() {
		room, err := p.dbClient.RoomByName(ctx, roomName)
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

func (p *Plexams) setPrePlannedRooms(prepareRoomsCfg *prepareRoomsCfg, reporter Reporter) []*model.PlannedRoom {
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
	for _, exam := range prepareRoomsCfg.exams {
		for _, prePlannedRoom := range prepareRoomsCfg.prePlannedRooms[exam.Exam.Ancode] {
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
	for _, exam := range prepareRoomsCfg.exams {
		prePlannedRooms, ok := prepareRoomsCfg.prePlannedRooms[exam.Exam.Ancode]
		if !ok {
			continue
		}
		exclusive := make([]*model.PrePlannedRoom, 0, len(prePlannedRooms))
		for _, prePlannedRoom := range prePlannedRooms {
			if !isShared(prePlannedRoom) {
				exclusive = append(exclusive, prePlannedRoom)
			}
		}
		examRooms = append(examRooms, p.assignPrePlannedRooms(prepareRoomsCfg, exam, exclusive, seatsTakenMap, reporter)...)
	}

	if sharedRooms.Cardinality() == 0 {
		return examRooms
	}

	// Pass 2: shared pre-planned rooms last, larger exam (more remaining normal
	// regs) first, so that if the shared room is too small the bigger exam gets
	// the seats and the overflow of the smaller one falls through to the normal
	// room allocation.
	examsBySize := make([]*model.ExamWithRegsAndRooms, len(prepareRoomsCfg.exams))
	copy(examsBySize, prepareRoomsCfg.exams)
	sort.SliceStable(examsBySize, func(i, j int) bool {
		return len(examsBySize[i].NormalRegsMtknr) > len(examsBySize[j].NormalRegsMtknr)
	})

	for _, exam := range examsBySize {
		prePlannedRooms, ok := prepareRoomsCfg.prePlannedRooms[exam.Exam.Ancode]
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
		examRooms = append(examRooms, p.assignPrePlannedRooms(prepareRoomsCfg, exam, shared, seatsTakenMap, reporter)...)
	}

	return examRooms
}

// assignPrePlannedRooms fills the given pre-planned rooms of a single exam,
// honoring the slot-wide seatsTakenMap so that rooms shared between concurrent
// exams are not overbooked.
func (p *Plexams) assignPrePlannedRooms(prepareRoomsCfg *prepareRoomsCfg, exam *model.ExamWithRegsAndRooms,
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
		room, ok := prepareRoomsCfg.roomInfo[prePlannedRoom.RoomName]
		if !ok {
			log.Error().Str("roomName", prePlannedRoom.RoomName).Msg("pre-planned room not found in room info")
			panic(fmt.Sprintf("pre-planned room %s not found in room info", prePlannedRoom.RoomName))
		}

		// a room blocked for this slot wins over the pre-planning: skip it.
		slotKey := SlotNumber{day: exam.Exam.PlanEntry.DayNumber, slot: exam.Exam.PlanEntry.SlotNumber}
		if blocked, ok := prepareRoomsCfg.blockedRooms[slotKey]; ok && blocked.Contains(prePlannedRoom.RoomName) {
			reporter.Warnf(aurora.Sprintf(
				aurora.Red("pre-planned room %s for %d is blocked in slot (%d,%d); skipped"),
				prePlannedRoom.RoomName, exam.Exam.Ancode, slotKey.day, slotKey.slot))
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

			// A shared room may already be full (taken by another exam) when this
			// exam gets to it; don't create an empty room entry then (reserve rooms
			// are intentional placeholders and are kept).
			if studentCountInRoom == 0 && !prePlannedRoom.Reserve {
				continue
			}

			studentsInRoom := exam.NormalRegsMtknr[:studentCountInRoom]
			exam.NormalRegsMtknr = exam.NormalRegsMtknr[studentCountInRoom:]

			examRoom = &model.PlannedRoom{
				Day:            exam.Exam.PlanEntry.DayNumber,
				Slot:           exam.Exam.PlanEntry.SlotNumber,
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
				Day:               exam.Exam.PlanEntry.DayNumber,
				Slot:              exam.Exam.PlanEntry.SlotNumber,
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
		p.addPlannedRoom(prepareRoomsCfg, exam, room, examRoom)

		examRooms = append(examRooms, examRoom)
	}
	reporter.Println(aurora.Sprintf(aurora.Green("added %d room(s)"), len(prePlannedRooms)))

	return examRooms
}

func (p *Plexams) addPlannedRoom(prepareRoomsCfg *prepareRoomsCfg, exam *model.ExamWithRegsAndRooms, room *model.Room, examRoom *model.PlannedRoom) {
	exam.Rooms = append(exam.Rooms, examRoom)
	if !examRoom.HandicapRoomAlone && room.Seats-len(examRoom.StudentsInRoom)-len(exam.NtasInNormalRooms) > 0 {
		if plannedRooms, ok := prepareRoomsCfg.plannedRoomsWithFreeSeats[room.Name]; ok {
			plannedRooms.plannedRooms = append(plannedRooms.plannedRooms, examRoom)
			plannedRooms.freeSeats -= (len(examRoom.StudentsInRoom) + len(exam.NtasInNormalRooms))
		} else {
			prepareRoomsCfg.plannedRoomsWithFreeSeats[room.Name] = &plannedRoomsWithFreeSeats{
				plannedRooms: []*model.PlannedRoom{examRoom},
				freeSeats:    room.Seats - (len(examRoom.StudentsInRoom) + len(exam.NtasInNormalRooms)),
			}
		}
	}
	// Remove the room from availableRooms
	for i, r := range prepareRoomsCfg.availableRooms {
		if r.Name == room.Name {
			prepareRoomsCfg.availableRooms = append(prepareRoomsCfg.availableRooms[:i], prepareRoomsCfg.availableRooms[i+1:]...)
			break
		}
	}
}

func (p *Plexams) findRoomWithFreeSeats(prepareRoomsCfg *prepareRoomsCfg, exam *model.ExamWithRegsAndRooms, neededSeats int) *model.Room {
	if len(prepareRoomsCfg.plannedRoomsWithFreeSeats) == 0 {
		return nil
	}

OUTER:
	for roomName, plannedRoomWithFreeSeats := range prepareRoomsCfg.plannedRoomsWithFreeSeats {
		if plannedRoomWithFreeSeats.freeSeats >= neededSeats {
			room := prepareRoomsCfg.roomInfo[roomName]
			if roomSatisfiesConstraints(room, exam.Exam.Constraints) {
				for _, plannedRoom := range plannedRoomWithFreeSeats.plannedRooms {
					otherExam := prepareRoomsCfg.examsMap[plannedRoom.Ancode]
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

func roomSatisfiesConstraints(room *model.Room, constraints *model.Constraints) bool {
	if constraints == nil || constraints.RoomConstraints == nil {
		// room without constraints should be no lab!
		return !room.Exahm && !room.Lab && !room.Seb
	}
	if constraints.RoomConstraints.Exahm && !room.Exahm {
		return false
	}
	if constraints.RoomConstraints.Lab && !room.Lab {
		return false
	}
	if constraints.RoomConstraints.PlacesWithSocket && !room.PlacesWithSocket {
		return false
	}
	if constraints.RoomConstraints.Seb && !room.Seb {
		return false
	}
	if constraints.RoomConstraints.AllowedRooms != nil && !set.NewSet(constraints.RoomConstraints.AllowedRooms...).Contains(room.Name) {
		return false
	}

	return true
}

func findRoom(prepareRoomsCfg *prepareRoomsCfg, exam *model.ExamWithRegsAndRooms) *model.Room {
	for i, room := range prepareRoomsCfg.availableRooms {
		if roomSatisfiesConstraints(room, exam.Exam.Constraints) {
			// remove room from available rooms
			prepareRoomsCfg.availableRooms = append(prepareRoomsCfg.availableRooms[:i], prepareRoomsCfg.availableRooms[i+1:]...)
			return room
		}
	}
	return nil
}

func findSmallestRoom(prepareRoomsCfg *prepareRoomsCfg, exam *model.ExamWithRegsAndRooms) *model.Room {
	for i := len(prepareRoomsCfg.availableRooms) - 1; i >= 0; i-- {
		room := prepareRoomsCfg.availableRooms[i]
		if roomSatisfiesConstraints(room, exam.Exam.Constraints) {
			// remove room from available rooms
			prepareRoomsCfg.availableRooms = append(prepareRoomsCfg.availableRooms[:i], prepareRoomsCfg.availableRooms[i+1:]...)
			return room
		}
	}
	return nil
}
