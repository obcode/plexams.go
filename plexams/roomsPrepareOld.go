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

func (p *Plexams) RoomsForNTAsWithRoomAloneOld() error {
	ctx := context.Background()
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	ntasMap := make(map[string]*model.Student)
	type dayEntry struct {
		slot   int
		ancode int
		mtknr  string
	}
	daysMap := make(map[int][]dayEntry)
	examsMap := make(map[int]*model.PlannedExam)

	plannedRooms := make([]*model.PlannedRoom, 0)

	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            "",
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	for _, nta := range ntas {
		if !nta.Nta.NeedsRoomAlone {
			continue
		}

		cfg.Suffix = aurora.Sprintf(aurora.Cyan(" finding exams for %s (%s)"),
			aurora.Yellow(nta.Name),
			aurora.Green(nta.Mtknr),
		)
		spinner, err := yacspin.New(cfg)
		if err != nil {
			log.Debug().Err(err).Msg("cannot create spinner")
		}
		err = spinner.Start()
		if err != nil {
			log.Debug().Err(err).Msg("cannot start spinner")
		}

		ntasMap[nta.Mtknr] = nta

		regsNew := make([]int, 0, len(nta.Regs))
		for _, ancode := range nta.Regs {
			exam, err := p.PlannedExam(ctx, ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam")
				return err
			}
			if exam.Constraints == nil || !exam.Constraints.NotPlannedByMe {
				examsMap[exam.Ancode] = exam
				dayEntries, ok := daysMap[exam.PlanEntry.DayNumber]
				if !ok {
					dayEntries = make([]dayEntry, 0, 1)
				}
				daysMap[exam.PlanEntry.DayNumber] = append(dayEntries, dayEntry{
					slot:   exam.PlanEntry.SlotNumber,
					ancode: ancode,
					mtknr:  nta.Mtknr,
				})
				regsNew = append(regsNew, ancode)
			}
		}

		nta.Regs = regsNew

		spinner.StopMessage(aurora.Sprintf(aurora.Cyan("found %v"),
			aurora.Magenta(nta.Regs),
		))

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	days := make([]int, 0, len(daysMap))
	for day := range daysMap {
		days = append(days, day)
	}
	sort.Ints(days)

	for _, day := range days {

		slotsMap := make(map[int][]dayEntry)
		for _, entry := range daysMap[day] {
			slotEntries, ok := slotsMap[entry.slot]
			if !ok {
				slotEntries = make([]dayEntry, 0, 1)
			}
			slotsMap[entry.slot] = append(slotEntries, entry)
		}

		slots := make([]int, 0, len(slotsMap))
		for slot := range slotsMap {
			slots = append(slots, slot)
		}
		sort.Ints(slots)

		prevSlot := -100
		roomsInPrevSlot := set.NewSet[string]()
		for _, slot := range slots {
			roomsInSlot := set.NewSet[string]()
			if prevSlot+1 < slot {
				roomsInPrevSlot = set.NewSet[string]()
			}

			slotEntries := slotsMap[slot]
			rooms, err := p.RoomsForSlot(ctx, day, slot)
			if err != nil {
				log.Error().Err(err).Int("day", day).Int("slot", slotEntries[0].slot).Msg("no rooms for slot found")
			}

			cfg.Suffix = aurora.Sprintf(aurora.Cyan(" slot (%d/%d) with %d needed room(s)"), day, slot, len(slotEntries))
			spinner, err := yacspin.New(cfg)
			if err != nil {
				log.Debug().Err(err).Msg("cannot create spinner")
			}
			err = spinner.Start()
			if err != nil {
				log.Debug().Err(err).Msg("cannot start spinner")
			}

			for _, slotEntry := range slotEntries {

				var room *model.Room
				exam := examsMap[slotEntry.ancode]
				if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil && exam.Constraints.RoomConstraints.Exahm {
					for _, exahmRoom := range rooms.ExahmRooms {
						if exahmRoom.Handicap {
							room = exahmRoom
							break
						}
					}
					if room == nil {
						fmt.Printf("we need an exahm room!!!")
						return fmt.Errorf("we need an exahm room")
					}
				} else {
					for _, ntaRoom := range rooms.NtaRooms {
						if roomsInPrevSlot.Contains(ntaRoom.Name) || roomsInSlot.Contains(ntaRoom.Name) {
							continue
						}
						room = ntaRoom
						break
					}
				}
				roomsInSlot.Add(room.Name)

				nta := ntasMap[slotEntry.mtknr]

				ntaDuration := int(math.Ceil(float64(exam.ZpaExam.Duration*(100+nta.Nta.DeltaDurationPercent)) / 100))

				plannedRooms = append(plannedRooms, &model.PlannedRoom{
					Day:               day,
					Slot:              slot,
					RoomName:          room.Name,
					Ancode:            slotEntry.ancode,
					Duration:          ntaDuration,
					Handicap:          true,
					HandicapRoomAlone: true,
					Reserve:           false,
					StudentsInRoom:    []string{nta.Nta.Mtknr},
					NtaMtknr:          &nta.Nta.Mtknr,
				})
			}

			prevSlot = slot
			roomsInPrevSlot = roomsInSlot

			spinner.StopMessage(aurora.Sprintf(aurora.Green("using %v"), roomsInSlot.ToSlice()))
			err = spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
		}
		// }
	}

	for _, plannedRoom := range plannedRooms {
		exam := examsMap[plannedRoom.Ancode]
		fmt.Println(aurora.Sprintf(aurora.Cyan("(%d/%d) %d. %s (%s), Raum %s für %s (%d Minuten)"), plannedRoom.Day, plannedRoom.Slot,
			exam.Ancode, aurora.Blue(exam.ZpaExam.Module), exam.ZpaExam.MainExamer,
			aurora.Magenta(plannedRoom.RoomName), aurora.Green(ntasMap[*plannedRoom.NtaMtknr].Name), aurora.Green(plannedRoom.Duration),
		))
	}
	return p.dbClient.ReplaceRoomsForNTA(ctx, plannedRooms)
}

// TODO: rewrite me.
func (p *Plexams) PrepareRoomForExamsOld() error {
	ctx := context.Background()

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

	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}

	roomInfo := make(map[string]*model.Room)
	for _, room := range allRooms {
		roomInfo[room.Name] = room
	}

	// only if room is needed more than 100 Minutes
	roomsNotUsableInSlot := set.NewSet[string]()

	examRooms := make([]*model.PlannedRoom, 0)
	for _, slot := range p.semesterConfig.Slots {
		cfg := yacspin.Config{
			Frequency: 100 * time.Millisecond,
			CharSet:   yacspin.CharSets[69],
			Suffix: aurora.Sprintf(aurora.Black("finding rooms for slot (%d/%d)"),
				aurora.Yellow(slot.DayNumber),
				aurora.Yellow(slot.SlotNumber),
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

		log.Debug().Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Msg("preparing rooms for slot")
		// get exams
		if slot.SlotNumber == 1 {
			roomsNotUsableInSlot = set.NewSet[string]()
		}
		examsInPlan, err := p.GetExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)

		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to find exams in slot")
			return err
		}

		// no exams in slot
		if len(examsInPlan) == 0 {
			spinner.StopMessage(aurora.Sprintf(aurora.Blue("no exams in slot")))
			err := spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
			roomsNotUsableInSlot = set.NewSet[string]()
			continue
		}

		// no exams for me to plan in slot
		needRooms := false
		for _, exam := range examsInPlan {
			if exam.Constraints == nil || !exam.Constraints.NotPlannedByMe {
				needRooms = true
				break
			}
		}

		if !needRooms {
			spinner.StopMessage(aurora.Sprintf(aurora.Blue("no exams for me to plan in slot")))
			err = spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
			roomsNotUsableInSlot = set.NewSet[string]()
			continue
		}

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}

		// planning for each exam starts here
		exams := make([]*model.ExamWithRegsAndRooms, 0, len(examsInPlan))
		examsMap := make(map[int]*model.PlannedExam)
		for _, examInPlan := range examsInPlan {

			if examInPlan.Constraints != nil && examInPlan.Constraints.NotPlannedByMe {
				continue
			}

			ntas := examInPlan.Ntas
			ntaMtknrs := set.NewSet[string]()
			ntasInNormalRooms := make([]*model.NTA, 0)
			for _, nta := range ntas {
				ntaMtknrs.Add(nta.Mtknr)
				if !nta.NeedsRoomAlone {
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

			addSeats, ok := additionalSeats[examInPlan.Ancode]
			if ok {
				fmt.Println(aurora.Sprintf(aurora.BrightRed("   adding %d seats to %d. %s (%s)"),
					addSeats, examInPlan.Ancode, examInPlan.ZpaExam.Module, examInPlan.ZpaExam.MainExamer))
				for i := 0; i < addSeats; i++ {
					normalRegs = append(normalRegs, "dummy")
				}
			}

			exams = append(exams, &model.ExamWithRegsAndRooms{
				Exam:            examInPlan,
				NormalRegsMtknr: normalRegs,
				Ntas:            ntasInNormalRooms,
				Rooms:           make([]*model.PlannedRoom, 0),
			})
			examsMap[examInPlan.Ancode] = examInPlan
		}

		// get rooms
		slotWithRooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to get rooms for slot")
			return err
		}

		if roomsNotUsableInSlot.Cardinality() > 0 {
			normalRooms := make([]*model.Room, 0, len(slotWithRooms.NormalRooms))
			for _, normalRoom := range slotWithRooms.NormalRooms {
				if !roomsNotUsableInSlot.Contains(normalRoom.Name) {
					normalRooms = append(normalRooms, normalRoom)
				}
			}
			slotWithRooms.NormalRooms = normalRooms

			exahmRooms := make([]*model.Room, 0, len(slotWithRooms.ExahmRooms))
			for _, exahmRoom := range slotWithRooms.ExahmRooms {
				if !roomsNotUsableInSlot.Contains(exahmRoom.Name) {
					exahmRooms = append(exahmRooms, exahmRoom)
				}
			}
			slotWithRooms.ExahmRooms = exahmRooms

			labRooms := make([]*model.Room, 0, len(slotWithRooms.LabRooms))
			for _, labRoom := range slotWithRooms.LabRooms {
				if !roomsNotUsableInSlot.Contains(labRoom.Name) {
					labRooms = append(labRooms, labRoom)
				}
			}
			slotWithRooms.LabRooms = labRooms

			roomsNotUsableInSlot = set.NewSet[string]()
		}

		type PlannedRoomsWithFreeSeats struct {
			rooms     []*model.PlannedRoom
			freeSeats int
		}
		plannedRoomsWithFreeSeats := make(map[string]PlannedRoomsWithFreeSeats)

		// rooms without NTA
		for {
			if len(exams) == 0 {
				break
			}

			sort.Slice(exams, func(i, j int) bool {
				return len(exams[i].NormalRegsMtknr)+len(exams[i].Ntas) > len(exams[j].NormalRegsMtknr)+len(exams[j].Ntas)
			})

			if len(exams[0].NormalRegsMtknr) == 0 {
				break
			}

			exam := exams[0]
			exams = exams[1:]

			cfg.Suffix = aurora.Sprintf(aurora.Magenta(" ↪ %d. %s (%s): %d of %d studs left"),
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

			neededSeats := len(exam.NormalRegsMtknr) + len(exam.Ntas)

			if neededSeats < 10 {

				type RoomWithSeatsLeft struct {
					room      *model.Room
					seatsLeft int
				}
				var roomFound *RoomWithSeatsLeft
			OUTER:
				for _, plannedRoomWithFreeSeats := range plannedRoomsWithFreeSeats {
					seatsLeft := plannedRoomWithFreeSeats.freeSeats - neededSeats
					if seatsLeft <= 0 {
						continue
					}
					for _, room := range plannedRoomWithFreeSeats.rooms {
						roomInfo := roomInfo[room.RoomName]
						otherExam := examsMap[room.Ancode]
						if exam.Exam.Constraints != nil && exam.Exam.Constraints.RoomConstraints != nil {
							if exam.Exam.Constraints.RoomConstraints.Exahm && !roomInfo.Exahm ||
								exam.Exam.Constraints.RoomConstraints.Lab && !roomInfo.Lab ||
								exam.Exam.Constraints.RoomConstraints.PlacesWithSocket && !roomInfo.PlacesWithSocket ||
								exam.Exam.Constraints.RoomConstraints.Seb && !roomInfo.Seb {
								continue OUTER
							}
						}
						if exam.Exam.ZpaExam.Duration != otherExam.ZpaExam.Duration {
							continue OUTER
						}

						if exam.Exam.ZpaExam.MainExamerID == otherExam.ZpaExam.MainExamerID {
							roomFound = &RoomWithSeatsLeft{
								room:      roomInfo,
								seatsLeft: plannedRoomWithFreeSeats.freeSeats - neededSeats,
							}
							break OUTER
						}
						if exam.Exam.ZpaExam.Module == otherExam.ZpaExam.Module {
							roomFound = &RoomWithSeatsLeft{
								room:      roomInfo,
								seatsLeft: plannedRoomWithFreeSeats.freeSeats - neededSeats,
							}
							break OUTER
						}

						if roomFound == nil || roomFound.seatsLeft < seatsLeft {
							roomFound = &RoomWithSeatsLeft{
								room:      roomInfo,
								seatsLeft: seatsLeft,
							}
						}
					}
				}
				if roomFound != nil {
					room = roomFound.room
				}
			}

			if room == nil {
				// TODO: room used to long in previous slot
				if exam.Exam.Constraints != nil {
					if exam.Exam.Constraints.Online {
						room = &model.Room{
							Name:  "ONLINE",
							Seats: 1000,
						}
					} else if exam.Exam.Constraints.RoomConstraints != nil {
						if exam.Exam.Constraints.RoomConstraints.Exahm {
							if len(slotWithRooms.ExahmRooms) > 0 {
								room = slotWithRooms.ExahmRooms[0]
								slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
							}
						} else if exam.Exam.Constraints.RoomConstraints.Seb {
							if len(slotWithRooms.ExahmRooms) > 0 {
								room = slotWithRooms.ExahmRooms[0]
								slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
							} else if len(slotWithRooms.LabRooms) > 0 {
								room = slotWithRooms.LabRooms[0]
								slotWithRooms.LabRooms = slotWithRooms.LabRooms[1:]
							}
						} else if exam.Exam.Constraints.RoomConstraints.Lab {
							if len(slotWithRooms.LabRooms) > 0 {
								room = slotWithRooms.LabRooms[0]
								slotWithRooms.LabRooms = slotWithRooms.LabRooms[1:]
							}
						} else if exam.Exam.Constraints.RoomConstraints.PlacesWithSocket {
							for i := 0; i < len(slotWithRooms.NormalRooms); i++ {
								if slotWithRooms.NormalRooms[i].PlacesWithSocket {
									room = slotWithRooms.NormalRooms[i]
									slotWithRooms.NormalRooms = append(slotWithRooms.NormalRooms[:i], slotWithRooms.NormalRooms[i+1:]...)
									break
								}
							}
						} else {
							room = slotWithRooms.NormalRooms[0]
							slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
						}
					} else {
						room = slotWithRooms.NormalRooms[0]
						slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
					}
				} else {
					room = slotWithRooms.NormalRooms[0]
					slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
				}
			}

			if room == nil {
				log.Error().Int("ancode", exam.Exam.Ancode).
					Msg("no room found for exam")
				room = &model.Room{
					Name:  "No Room",
					Seats: 1000,
				}
			}

			// TODO: no if only room for exam
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

			examRoom := model.PlannedRoom{
				Day:               slot.DayNumber,
				Slot:              slot.SlotNumber,
				RoomName:          room.Name,
				Ancode:            exam.Exam.Ancode,
				Duration:          exam.Exam.ZpaExam.Duration,
				Handicap:          false,
				HandicapRoomAlone: false,
				Reserve:           reserveRoom,
				StudentsInRoom:    studentsInRoom,
				NtaMtknr:          nil,
			}

			exam.Rooms = append(exam.Rooms, &examRoom)
			examRooms = append(examRooms, &examRoom) // nolint

			exams = append(exams, exam)

			// for _, plannedRoom := range exam.Rooms {
			plannedRoomWithFreeSeats, ok := plannedRoomsWithFreeSeats[examRoom.RoomName]
			if !ok {
				plannedRoomWithFreeSeats = PlannedRoomsWithFreeSeats{
					rooms:     make([]*model.PlannedRoom, 0, 1),
					freeSeats: roomInfo[examRoom.RoomName].Seats,
				}
			}
			plannedRoomsWithFreeSeats[examRoom.RoomName] = PlannedRoomsWithFreeSeats{
				rooms:     append(plannedRoomWithFreeSeats.rooms, &examRoom),
				freeSeats: plannedRoomWithFreeSeats.freeSeats - (len(examRoom.StudentsInRoom) + len(exam.Ntas)),
			}
			// }

			spinner.StopMessage(aurora.Sprintf(aurora.Green("added %s for %d students (max. %d)"),
				examRoom.RoomName, len(examRoom.StudentsInRoom), room.Seats))
			err = spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
		} // for exams

		// NTAs in normal rooms
		for _, exam := range exams {
			maxDuration := exam.Exam.ZpaExam.Duration
			for _, nta := range exam.Ntas {
				ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
				if maxDuration < ntaDuration {
					maxDuration = ntaDuration
				}
			}

			// find room with a seat left
			var roomFound *model.Room
			for _, room := range exam.Rooms {
				if maxDuration > 100 && (room.RoomName == "R1.046" || room.RoomName == "R1.049") {
					continue
				}

				if roomInfo[room.RoomName].Seats >= len(room.StudentsInRoom)+len(exam.Ntas) {
					roomFound = roomInfo[room.RoomName]
					break
				}
			}
			if roomFound == nil {
				// need a new room
				if exam.Exam.Constraints != nil {
					if exam.Exam.Constraints.Online {
						roomFound = &model.Room{
							Name:  "ONLINE",
							Seats: 1000,
						}
					} else if exam.Exam.Constraints.RoomConstraints != nil {
						if exam.Exam.Constraints.RoomConstraints.Exahm {
							if len(slotWithRooms.ExahmRooms) > 0 {
								roomFound = slotWithRooms.ExahmRooms[0]
								slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
							}
						} else if exam.Exam.Constraints.RoomConstraints.Seb {
							if len(slotWithRooms.ExahmRooms) > 0 {
								roomFound = slotWithRooms.ExahmRooms[0]
								slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
							}
						} else if exam.Exam.Constraints.RoomConstraints.Lab {
							if len(slotWithRooms.LabRooms) > 0 {
								roomFound = slotWithRooms.LabRooms[0]
								slotWithRooms.LabRooms = slotWithRooms.LabRooms[1:]
							}
						} else if exam.Exam.Constraints.RoomConstraints.PlacesWithSocket {
							for i := 0; i < len(slotWithRooms.NormalRooms); i++ {
								if slotWithRooms.NormalRooms[i].PlacesWithSocket {
									roomFound = slotWithRooms.NormalRooms[i]
									slotWithRooms.NormalRooms = append(slotWithRooms.NormalRooms[:i], slotWithRooms.NormalRooms[i+1:]...)
									break
								}
							}
						} else {
							roomFound = slotWithRooms.NormalRooms[0]
							slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
						}
					} else {
						roomFound = slotWithRooms.NormalRooms[0]
						slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
					}
				} else {
					roomFound = slotWithRooms.NormalRooms[0]
					slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
				}
			}

			for _, nta := range exam.Ntas {
				ntaDuration := int(math.Ceil(float64(exam.Exam.ZpaExam.Duration*(100+nta.DeltaDurationPercent)) / 100))
				examRoom := model.PlannedRoom{
					Day:               slot.DayNumber,
					Slot:              slot.SlotNumber,
					RoomName:          roomFound.Name,
					Ancode:            exam.Exam.Ancode,
					Duration:          ntaDuration,
					Handicap:          true,
					HandicapRoomAlone: false,
					Reserve:           false,
					StudentsInRoom:    []string{nta.Mtknr},
					NtaMtknr:          &nta.Mtknr,
				}

				exam.Rooms = append(exam.Rooms, &examRoom)
				examRooms = append(examRooms, &examRoom)
				fmt.Println(aurora.Sprintf(aurora.Red("   adding NTA room %s for %s (%d minuntes)"),
					aurora.Green(roomFound.Name), aurora.Green(nta.Name), aurora.Green(ntaDuration)))

				if ntaDuration > 100 {
					roomsNotUsableInSlot.Add(roomFound.Name)
					fmt.Println(aurora.Sprintf(aurora.Red("   room %s not usable in next slot!"),
						aurora.Green(roomFound.Name)))
				}
			}
		}

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	} // for slot

	// err := p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "rooms_for_exams"), examRooms)
	// if err != nil {
	// 	log.Error().Err(err).Msg("cannot save rooms for exams")
	// 	return err
	// }

	return p.dbClient.ReplaceNonNTARooms(ctx, examRooms)
}

// FIXME: rewrite me
func (p *Plexams) GetRoomsForNTAOld(name string) error {
	// 	ctx := context.Background()
	// 	ntas, err := p.NtasWithRegs(ctx)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	var nta *model.NTAWithRegs
	// 	for _, ntaInDB := range ntas {
	// 		if strings.HasPrefix(ntaInDB.Nta.Name, name) {
	// 			nta = ntaInDB
	// 			break
	// 		}
	// 	}
	// 	if nta == nil {
	// 		return fmt.Errorf("NTA with name=%s not found", name)
	// 	}
	// 	log.Debug().Str("name", nta.Nta.Name).Msg("found nta")

	// ANCODES:
	// 	for _, ancode := range nta.Regs.Ancodes {
	// 		exam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get zpa exam")
	// 		}

	// 		constraints, err := p.ConstraintForAncode(ctx, ancode)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get constraints")
	// 		}
	// 		if constraints != nil && constraints.NotPlannedByMe {
	// 			log.Debug().Int("ancode", ancode).Str("examer", exam.MainExamer).Str("module", exam.Module).Msg("exam not planned by me")
	// 			continue
	// 		}
	// 		log.Debug().Int("ancode", ancode).Str("examer", exam.MainExamer).Str("module", exam.Module).Msg("found exam")

	// 		roomsForExam, err := p.dbClient.RoomsForAncode(ctx, ancode)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get rooms")
	// 		}
	// 		for _, room := range roomsForExam {
	// 			for _, stud := range room.Students {
	// 				if nta.Nta.Mtknr == stud.Mtknr {
	// 					fmt.Printf("%d. %s: %s --- Raum %s\n", ancode, exam.MainExamer, exam.Module, room.RoomName)
	// 					continue ANCODES
	// 				}
	// 			}
	// 		}

	// 	}

	return fmt.Errorf("rewrite me")
}
