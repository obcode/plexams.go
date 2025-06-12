package plexams

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
)

func (p *Plexams) PrepareRoomForExams() error {
	ctx := context.Background()

	prepareRoomsCfg, err := p.prepareRoomsCfg(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get prepare rooms config")
		return err
	}

	examRooms := make([]*model.PlannedRoom, 0)
	for _, slot := range p.semesterConfig.Slots {
		prepareRoomsCfg.slot = slot
		rooms, err := p.prepareRoomsForExamsInSlot(ctx, prepareRoomsCfg)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).
				Msg("error while preparing rooms for exams in slot")
			continue
		}
		examRooms = append(examRooms, rooms...)
	}

	return p.dbClient.ReplaceNonNTARooms(ctx, examRooms)
}

func (p *Plexams) prepareRoomsForExamsInSlot(ctx context.Context, prepareRoomsCfg *prepareRoomsCfg) ([]*model.PlannedRoom, error) {
	cfg := yacspin.Config{
		Frequency: 100 * time.Millisecond,
		CharSet:   yacspin.CharSets[69],
		Suffix: aurora.Sprintf(aurora.Black("preparing data for slot (%d/%d)"),
			aurora.Yellow(prepareRoomsCfg.slot.DayNumber),
			aurora.Yellow(prepareRoomsCfg.slot.SlotNumber),
		),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	log.Debug().Int("day", prepareRoomsCfg.slot.DayNumber).Int("slot", prepareRoomsCfg.slot.SlotNumber).Msg("preparing rooms for slot")

	examsInSlot, err := p.ExamsInSlot(ctx, prepareRoomsCfg.slot.DayNumber, prepareRoomsCfg.slot.SlotNumber)
	if err != nil {
		log.Error().Err(err).Int("day", prepareRoomsCfg.slot.DayNumber).Int("time", prepareRoomsCfg.slot.SlotNumber).
			Msg("error while trying to find exams in slot")
		return nil, err
	}

	if len(examsInSlot) == 0 {
		spinner.StopMessage(aurora.Sprintf(aurora.Blue("no exams in slot")))
		err := spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		prepareRoomsCfg.roomsNotUsableInSlot = set.NewSet[string]()
		return nil, nil
	}

	spinner.StopMessage(aurora.Sprintf(aurora.Blue("start planning")))

	err = spinner.Stop()
	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	if prepareRoomsCfg.slot.SlotNumber == 1 { // a new day
		prepareRoomsCfg.roomsNotUsableInSlot = set.NewSet[string]()
	}

	prepareRoomsCfg.exams, prepareRoomsCfg.examsMap = p.mkExamsMap(prepareRoomsCfg, examsInSlot)
	prepareRoomsCfg.availableRooms, err = p.availableRoomsInSlot(ctx, prepareRoomsCfg)

	if err != nil {
		log.Error().Err(err).Int("day", prepareRoomsCfg.slot.DayNumber).Int("time", prepareRoomsCfg.slot.SlotNumber).
			Msg("error while trying to get rooms for slot")
		return nil, err
	}

	prepareRoomsCfg.plannedRoomsWithFreeSeats = make(map[string]*plannedRoomsWithFreeSeats)

	examRooms := p.setPrePlannedRooms(prepareRoomsCfg)

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

		cfg.Suffix = aurora.Sprintf(aurora.Cyan(" ↪ %d. %s (%s): %d of %d studs left"),
			exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
			len(exam.NormalRegsMtknr), exam.Exam.StudentRegsCount)
		spinner, err := yacspin.New(cfg)
		if err != nil {
			log.Debug().Err(err).Msg("cannot create spinner")
		}
		err = spinner.Start()
		if err != nil {
			log.Debug().Err(err).Msg("cannot start spinner")
		}

		var room *model.Room
		neededSeats := len(exam.NormalRegsMtknr) + len(exam.NtasInNormalRooms)
		if neededSeats < 10 { // if we need less than 10 seats, we can use a room with free seats
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

		spinner.StopMessage(aurora.Sprintf(aurora.Green("added %s for %d students (max. %d)"),
			examRoom.RoomName, len(examRoom.StudentsInRoom), room.Seats))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	// rooms for NTAs in normal rooms
	for _, exam := range prepareRoomsCfg.exams {
		if len(exam.NtasInNormalRooms) == 0 {
			continue
		}

		cfg.Suffix = aurora.Sprintf(aurora.Magenta(" ↪ %d. %s (%s): %d students with NTA in normal rooms"),
			exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
			len(exam.NtasInNormalRooms))
		spinner, err := yacspin.New(cfg)
		if err != nil {
			log.Debug().Err(err).Msg("cannot create spinner")
		}
		err = spinner.Start()
		if err != nil {
			log.Debug().Err(err).Msg("cannot start spinner")
		}

		maxDuration := exam.Exam.ZpaExam.Duration
		for _, nta := range exam.NtasInNormalRooms {
			ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
			if maxDuration < ntaDuration {
				maxDuration = ntaDuration
			}
		}
		var room *model.Room
		for _, plannedRoom := range exam.Rooms {
			if maxDuration > 100 && (plannedRoom.RoomName == "R1.046" || plannedRoom.RoomName == "R1.049") {
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
		spinner.StopMessage(aurora.Sprintf(aurora.Green("added %s for %d students with NTA (%d minuntes)%s"),
			room.Name, len(exam.NtasInNormalRooms), maxDuration, comment))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	// rooms for NTAs in alone rooms
	for _, exam := range prepareRoomsCfg.exams {
		if len(exam.NtasInAloneRooms) == 0 {
			continue
		}

		for _, nta := range exam.NtasInAloneRooms {
			cfg.Suffix = aurora.Sprintf(aurora.Magenta(" ↪ %d. %s (%s): planning room for %s (%s)."),
				exam.Exam.Ancode, exam.Exam.ZpaExam.Module, exam.Exam.ZpaExam.MainExamer,
				nta.Name, nta.Mtknr)
			spinner, err := yacspin.New(cfg)
			if err != nil {
				log.Debug().Err(err).Msg("cannot create spinner")
			}
			err = spinner.Start()
			if err != nil {
				log.Debug().Err(err).Msg("cannot start spinner")
			}

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
			spinner.StopMessage(aurora.Sprintf(aurora.Green("added %s (%d minuntes)%s"),
				room.Name, ntaDuration, comment))
			err = spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
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

	prepareRoomsCfg := &prepareRoomsCfg{
		roomInfo:        roomInfo,
		prePlannedRooms: p.prePlannedRooms(ctx),
		additionalSeats: additionalSeats(),
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

func (p *Plexams) prePlannedRooms(ctx context.Context) map[int][]*model.PrePlannedRoom {
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

	return prePlannedRoomsMap
}

func (p *Plexams) mkExamsMap(prepareRoomsCfg *prepareRoomsCfg, examsInPlan []*model.PlannedExam) ([]*model.ExamWithRegsAndRooms, map[int]*model.PlannedExam) {
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
			fmt.Println(aurora.Sprintf(aurora.BrightRed("   adding %d seats to %d. %s (%s)"),
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

func (p *Plexams) setPrePlannedRooms(prepareRoomsCfg *prepareRoomsCfg) []*model.PlannedRoom {
	examRooms := make([]*model.PlannedRoom, 0)

	for _, exam := range prepareRoomsCfg.exams {
		prePlannedRooms, ok := prepareRoomsCfg.prePlannedRooms[exam.Exam.Ancode]
		if !ok {
			continue
		}
		cfg := yacspin.Config{
			Frequency: 100 * time.Millisecond,
			CharSet:   yacspin.CharSets[69],
			Suffix: aurora.Sprintf(aurora.Cyan("found preplanned rooms for %d. %s (%s)"),
				aurora.Magenta(exam.Exam.Ancode),
				aurora.Magenta(exam.Exam.ZpaExam.Module),
				aurora.Magenta(exam.Exam.ZpaExam.MainExamer)),
			SuffixAutoColon:   true,
			StopCharacter:     "✓",
			StopColors:        []string{"fgGreen"},
			StopFailMessage:   "error",
			StopFailCharacter: "✗",
			StopFailColors:    []string{"fgRed"},
		}

		spinner, err := yacspin.New(cfg)
		if err != nil {
			log.Debug().Err(err).Msg("cannot create spinner")
		}
		err = spinner.Start()
		if err != nil {
			log.Debug().Err(err).Msg("cannot start spinner")
		}
		for _, prePlannedRoom := range prePlannedRooms {
			room := findRoomByRoomName(prepareRoomsCfg.availableRooms, prePlannedRoom.RoomName)
			var examRoom *model.PlannedRoom
			if prePlannedRoom.Mtknr == nil { // room for normal students
				studentCountInRoom := room.Seats
				if studentCountInRoom > len(exam.NormalRegsMtknr) {
					studentCountInRoom = len(exam.NormalRegsMtknr)
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
				}
			} else { // room for NTA
				foundNTA := false
				for i, nta := range exam.NtasInNormalRooms {
					if nta.Mtknr == *prePlannedRoom.Mtknr {
						exam.NtasInNormalRooms = append(exam.NtasInNormalRooms[:i], exam.NtasInNormalRooms[i+1:]...)
						foundNTA = true
						break
					}
				}
				for i, nta := range exam.NtasInAloneRooms {
					if nta.Mtknr == *prePlannedRoom.Mtknr {
						exam.NtasInAloneRooms = append(exam.NtasInAloneRooms[:i], exam.NtasInAloneRooms[i+1:]...)
						foundNTA = true
						break
					}
				}
				if !foundNTA {
					log.Error().Str("mtknr", *prePlannedRoom.Mtknr).Msg("NTA not found in exam")
					continue
				}
				nta, err := p.dbClient.Nta(context.Background(), *prePlannedRoom.Mtknr)
				if err != nil {
					log.Debug().Err(err).Str("mtknr", *prePlannedRoom.Mtknr).Msg("cannot get NTA by MTKNR")
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
				}
			}
			p.addPlannedRoom(prepareRoomsCfg, exam, room, examRoom)

			examRooms = append(examRooms, examRoom)
		}
		spinner.StopMessage(aurora.Sprintf(aurora.Green("added %d room(s)"), len(prePlannedRooms)))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return examRooms
}

func (p *Plexams) addPlannedRoom(prepareRoomsCfg *prepareRoomsCfg, exam *model.ExamWithRegsAndRooms, room *model.Room, examRoom *model.PlannedRoom) {
	exam.Rooms = append(exam.Rooms, examRoom)
	if room.Seats-len(examRoom.StudentsInRoom)-len(exam.NtasInNormalRooms) > 0 {
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

func findRoomByRoomName(rooms []*model.Room, roomName string) *model.Room {
	for _, room := range rooms {
		if room.Name == roomName {
			return room
		}
	}
	return nil
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
