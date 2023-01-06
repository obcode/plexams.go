package plexams

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) PrepareRoomsForSemester() error {
	globalRooms, err := p.dbClient.GlobalRooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}

	roomsForSlots := make(map[int][]*model.Room)
	for _, room := range globalRooms {
		roomConstraints := viper.Get(fmt.Sprintf("roomConstraints.%s", room.Name))
		if roomConstraints == nil {
			fmt.Printf("%s: no constraints found\n", room.Name)
			for _, slot := range p.semesterConfig.Slots {
				slotNumber := slot.DayNumber*10 + slot.SlotNumber
				slotEntry, ok := roomsForSlots[slotNumber]
				if !ok {
					slotEntry = []*model.Room{room}
				} else {
					slotEntry = append(slotEntry, room)
				}
				roomsForSlots[slotNumber] = slotEntry
			}
		} else {
			allowedSlots := viper.Get(fmt.Sprintf("roomConstraints.%s.allowedSlots", room.Name))
			if allowedSlots != nil {
				fmt.Printf("%s: allowed slots found\n", room.Name)
				allowedSlotsSlice := allowedSlots.([]interface{})
				for _, allowedSlot := range allowedSlotsSlice {
					allowedSlotSlice := allowedSlot.([]interface{})
					slotNumber := allowedSlotSlice[0].(int)*10 + allowedSlotSlice[1].(int)
					slotEntry, ok := roomsForSlots[slotNumber]
					if !ok {
						slotEntry = []*model.Room{room}
					} else {
						slotEntry = append(slotEntry, room)
					}
					roomsForSlots[slotNumber] = slotEntry
				}
				// } else {
				// TODO: forbiddenSlots := viper.Get(fmt.Sprintf("roomConstraints.%s.forbiddenSlots", room.Name))
			}
		}
	}

	slotsWithRooms := make([]*model.SlotWithRooms, 0, len(roomsForSlots))
	for slot, rooms := range roomsForSlots {
		normalRooms, exahmRooms, labRooms, ntaRooms := splitRooms(rooms)
		slotsWithRooms = append(slotsWithRooms, &model.SlotWithRooms{
			DayNumber:   slot / 10,
			SlotNumber:  slot % 10,
			NormalRooms: normalRooms,
			ExahmRooms:  exahmRooms,
			LabRooms:    labRooms,
			NtaRooms:    ntaRooms,
		})
	}

	return p.dbClient.SaveRooms(context.Background(), slotsWithRooms)
}

func splitRooms(rooms []*model.Room) ([]*model.Room, []*model.Room, []*model.Room, []*model.Room) {
	normalRooms := make([]*model.Room, 0)
	exahmRooms := make([]*model.Room, 0)
	labRooms := make([]*model.Room, 0)
	ntaRooms := make([]*model.Room, 0)
	for _, room := range rooms {
		if room.Handicap {
			ntaRooms = append(ntaRooms, room)
		} else if room.Exahm {
			exahmRooms = append(exahmRooms, room)
		} else if room.Lab {
			labRooms = append(labRooms, room)
		} else {
			normalRooms = append(normalRooms, room)
		}
	}
	sort.Slice(normalRooms, func(i, j int) bool { return normalRooms[i].Seats > normalRooms[j].Seats })
	sort.Slice(exahmRooms, func(i, j int) bool { return exahmRooms[i].Seats > exahmRooms[j].Seats })
	sort.Slice(labRooms, func(i, j int) bool { return labRooms[i].Seats > labRooms[j].Seats })
	sort.Slice(ntaRooms, func(i, j int) bool { return ntaRooms[i].Seats < ntaRooms[j].Seats })
	return normalRooms, exahmRooms, labRooms, ntaRooms
}

func (p *Plexams) Rooms(ctx context.Context) ([]*model.Room, error) {
	return p.dbClient.Rooms(ctx)
}

func (p *Plexams) RoomsForSlot(ctx context.Context, day int, time int) (*model.SlotWithRooms, error) {
	return p.dbClient.RoomsForSlot(ctx, day, time)
}

func (p *Plexams) AddRoomToExam(ctx context.Context, input model.RoomForExamInput) (bool, error) {
	// room, err := p.getRoom(ctx, input.RoomName, input.Day, input.Time, input.SeatsPlanned)
	// if err != nil {
	// 	log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
	// 		Msg("cannot get room")
	// 	return false, err
	// }

	// err = p.dbClient.AddRoomToExam(ctx, &model.RoomForExam{
	// 	Ancode:       input.Ancode,
	// 	Room:         room,
	// 	SeatsPlanned: input.SeatsPlanned,
	// 	Duration:     input.Duration,
	// 	Handicap:     input.Handicap,
	// 	Mktnrs:       input.Mktnrs,
	// })
	// if err != nil {
	// 	log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
	// 		Msg("cannot save room to db")
	// 	return false, err
	// }

	// FIXME

	return false, nil
}

// func (p *Plexams) getRoom(ctx context.Context, roomName string, day, time, seatsNeeded int) (*model.Room, error) {
// 	roomsForSlot, err := p.RoomsForSlot(ctx, day, time)
// 	if err != nil {
// 		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get rooms for slot")
// 		return nil, err
// 	}
// 	var room *model.Room
// 	for _, roomForSlot := range roomsForSlot {
// 		if roomName == roomForSlot.Name {
// 			room = roomForSlot
// 			break
// 		}
// 	}
// 	if room == nil {
// 		return nil, fmt.Errorf("room %s for slot (%d,%d) not allowed", roomName, day, time)
// 	}

// 	roomAlreadyPlanned, err := p.dbClient.RoomPlannedInSlot(ctx, roomName, day, time)
// 	if err != nil {
// 		log.Error().Err(err).Str("room name", roomName).Int("day", day).Int("time", time).
// 			Msg("cannot get plannedrooms for slot")
// 		return nil, err
// 	}

// 	seatsUsedAlready := 0
// 	for _, roomInUse := range roomAlreadyPlanned {
// 		seatsUsedAlready += roomInUse.SeatsPlanned
// 	}

// 	if room.Seats-seatsUsedAlready < seatsNeeded {
// 		log.Debug().Str("room name", roomName).Int("day", day).Int("time", time).
// 			Msg("not enough seats left")
// 		return nil, fmt.Errorf("not enough seats left")
// 	}

// 	return room, nil
// }

func (p *Plexams) PrepareRoomForExams() error {
	ctx := context.Background()

	examRooms := make([]interface{}, 0)

	for _, slot := range p.semesterConfig.Slots {
		// get exams
		examsInPlan, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)

		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to find exams in slot")
			return err
		}

		exams := make([]*model.ExamWithRegsAndRooms, 0, len(examsInPlan))
		for _, examInPlan := range examsInPlan {
			ntas := examInPlan.Nta
			isNTA := func(studReg *model.StudentReg) bool {
				for _, nta := range ntas {
					if studReg.Mtknr == nta.Nta.Mtknr {
						return true
					}
				}
				return false
			}
			regs := make([]*model.StudentReg, 0)
			for _, registrations := range examInPlan.Exam.StudentRegs {
				for _, studReg := range registrations.StudentRegs {
					if !isNTA(studReg) {
						regs = append(regs, studReg)
					}
				}
			}

			exams = append(exams, &model.ExamWithRegsAndRooms{
				Exam:       examInPlan,
				NormalRegs: regs,
				NtaRegs:    ntas,
				Rooms:      make([]*model.RoomForExam, 0),
			})
		}

		// get rooms
		slotWithRooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to get rooms for slot")
			return err
		}

		// rooms without NTA
		for {
			if len(exams) == 0 {
				break
			}

			sort.Slice(exams, func(i, j int) bool {
				return len(exams[i].NormalRegs) > len(exams[j].NormalRegs)
			})

			if len(exams[0].NormalRegs) == 0 {
				break
			}

			exam := exams[0]
			exams = exams[1:]

			var room *model.Room

			if exam.Exam.Constraints != nil {
				if exam.Exam.Constraints.Online {
					room = &model.Room{
						Name:  "ONLINE",
						Seats: 1000,
					}
				} else if exam.Exam.Constraints.RoomConstraints != nil {
					if exam.Exam.Constraints.RoomConstraints.ExahmRooms {
						if len(slotWithRooms.ExahmRooms) > 0 {
							room = slotWithRooms.ExahmRooms[0]
							slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
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
					}
				} else {
					room = slotWithRooms.NormalRooms[0]
					slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
				}
			} else {
				room = slotWithRooms.NormalRooms[0]
				slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
			}

			if room == nil {
				log.Error().Int("ancode", exam.Exam.Exam.Ancode).
					Msg("no room found for exam")
				room = &model.Room{
					Name:  "No Room",
					Seats: 1000,
				}
			}

			reserveRoom := false
			studentCountInRoom := room.Seats
			if studentCountInRoom > len(exam.NormalRegs) {
				studentCountInRoom = len(exam.NormalRegs)
				if len(exam.Rooms) > 0 && studentCountInRoom < 10 {
					reserveRoom = true
				}
			}

			studentsInRoom := exam.NormalRegs[:studentCountInRoom]
			exam.NormalRegs = exam.NormalRegs[studentCountInRoom:]

			examRoom := model.RoomForExam{
				Ancode:       exam.Exam.Exam.Ancode,
				Room:         room,
				SeatsPlanned: len(studentsInRoom),
				Duration:     exam.Exam.Exam.ZpaExam.Duration,
				Handicap:     false,
				Reserve:      reserveRoom,
				Students:     studentsInRoom,
			}

			exam.Rooms = append(exam.Rooms, &examRoom)
			examRooms = append(examRooms, &examRoom)

			exams = append(exams, exam)
		} // for exams

		// NTAs
		for _, exam := range exams {
			if len(exam.NtaRegs) == 0 {
				continue
			}

			ntaRooms := slotWithRooms.NtaRooms

			for _, nta := range exam.NtaRegs {

				ntaDuration := int(math.Ceil(float64(exam.Exam.Exam.ZpaExam.Duration*(100+nta.Nta.DeltaDurationPercent)) / 100))

				if nta.Nta.NeedsRoomAlone {
					examRooms = append(examRooms, &model.RoomForExam{
						Ancode:       exam.Exam.Exam.Ancode,
						Room:         ntaRooms[0],
						SeatsPlanned: 1,
						Duration:     ntaDuration,
						Handicap:     true,
						Reserve:      false,
						Students: []*model.StudentReg{
							{
								Mtknr: nta.Nta.Mtknr,
								Name:  nta.Nta.Name,
							},
						},
					})
					ntaRooms = ntaRooms[1:]
				} else {
					// find room with a seat left
					for _, room := range exam.Rooms {
						if room.SeatsPlanned < room.Room.Seats {
							examRooms = append(examRooms, &model.RoomForExam{
								Ancode:       exam.Exam.Exam.Ancode,
								Room:         room.Room,
								SeatsPlanned: 1,
								Duration:     ntaDuration,
								Handicap:     true,
								Reserve:      false,
								Students: []*model.StudentReg{
									{
										Mtknr: nta.Nta.Mtknr,
										Name:  nta.Nta.Name,
									},
								},
							})
							break
						}
					}
				}
			}
		}
	} // for slot

	err := p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "rooms_for_exams"), examRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot save rooms for exams")
		return err
	}

	return nil
}

func (p *Plexams) ChangeRoom(ctx context.Context, ancode int, oldRoomName, newRoomName string) (bool, error) {
	roomsForAncode, err := p.dbClient.RoomsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting rooms for ancode")
		return false, err
	}

	var oldRoom *model.Room
	for _, room := range roomsForAncode {
		if room.Room.Name == oldRoomName {
			log.Debug().Msg("old room found")
			oldRoom = room.Room
		}
	}
	if oldRoom == nil {
		log.Error().Msg("old room not found")
		return false, fmt.Errorf("old room %s for ancode %d not found", oldRoomName, ancode)
	}

	slot, err := p.SlotForAncode(ctx, ancode)
	if err != nil || slot == nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting slot for ancode")
		return false, err
	}

	roomsForSlot, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
	if err != nil || slot == nil {
		log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
			Msg("error while getting rooms for slot")
		return false, err
	}

	var newRoom *model.Room

	if oldRoom.Exahm {
		for _, roomForSlot := range roomsForSlot.ExahmRooms {
			if roomForSlot.Name == newRoomName {
				newRoom = roomForSlot
			}
		}
	} else if oldRoom.Lab {
		for _, roomForSlot := range roomsForSlot.LabRooms {
			if roomForSlot.Name == newRoomName {
				newRoom = roomForSlot
			}
		}
	} else {
		for _, roomForSlot := range roomsForSlot.NormalRooms {
			if roomForSlot.Name == newRoomName {
				newRoom = roomForSlot
			}
		}
	}

	if newRoom == nil {
		log.Error().Msg("old room not found")
		return false, fmt.Errorf("new room %s for ancode %d not found", newRoomName, ancode)
	}

	return p.dbClient.ChangeRoom(ctx, ancode, oldRoom, newRoom)
}

func (p *Plexams) PlannedRoomNames(ctx context.Context) ([]string, error) {
	return p.dbClient.PlannedRoomNames(ctx)
}
