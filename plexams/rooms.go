package plexams

import (
	"context"
	"fmt"
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
		normalRooms, ntaRooms := splitRooms(rooms)
		slotsWithRooms = append(slotsWithRooms, &model.SlotWithRooms{
			DayNumber:  slot / 10,
			SlotNumber: slot % 10,
			Rooms:      normalRooms,
			NtaRooms:   ntaRooms,
		})
	}

	return p.dbClient.SaveRooms(context.Background(), slotsWithRooms)
}

func splitRooms(rooms []*model.Room) ([]*model.Room, []*model.Room) {
	normalRooms := make([]*model.Room, 0)
	ntaRooms := make([]*model.Room, 0)
	for _, room := range rooms {
		if room.Handicap {
			ntaRooms = append(ntaRooms, room)
		} else {
			normalRooms = append(normalRooms, room)
		}
	}
	sort.Slice(normalRooms, func(i, j int) bool { return normalRooms[i].Seats > normalRooms[j].Seats })
	sort.Slice(ntaRooms, func(i, j int) bool { return ntaRooms[i].Seats < ntaRooms[j].Seats })
	return normalRooms, ntaRooms
}

func (p *Plexams) Rooms(ctx context.Context) ([]*model.Room, error) {
	return p.dbClient.Rooms(ctx)
}

func (p *Plexams) RoomsForSlot(ctx context.Context, day int, time int) ([]*model.Room, error) {
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

		type examWithRegsAndRooms struct {
			exam       *model.ExamInPlan
			normalRegs []*model.StudentReg
			ntaRegs    []*model.NTAWithRegs
			rooms      []*model.RoomForExam
		}

		exams := make([]*examWithRegsAndRooms, 0, len(examsInPlan))
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

			exams = append(exams, &examWithRegsAndRooms{
				exam:       examInPlan,
				normalRegs: regs,
				ntaRegs:    ntas,
				rooms:      make([]*model.RoomForExam, 0),
			})
		}

		// get rooms
		rooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to get rooms for slot")
			return err
		}

		// normal rooms
		for {
			if len(exams) == 0 {
				break
			}

			sort.Slice(exams, func(i, j int) bool {
				return len(exams[i].normalRegs) > len(exams[j].normalRegs)
			})

			if len(exams[0].normalRegs) == 0 {
				break
			}

			// TODO: use exahm first, then lab, than placeswithsockets and then all other
			exam := exams[0]
			exams = exams[1:]

			var room *model.Room
			if exam.exam.Constraints != nil && exam.exam.Constraints.RoomConstraints != nil {
				if exam.exam.Constraints.RoomConstraints.ExahmRooms {
					for i := 0; i < len(rooms); i++ {
						if rooms[i].Exahm {
							room = rooms[i]
							rooms = append(rooms[:i], rooms[i+1:]...)
							break
						}
					}
				} else if exam.exam.Constraints.RoomConstraints.Lab {
					for i := 0; i < len(rooms); i++ {
						if rooms[i].Lab {
							room = rooms[i]
							rooms = append(rooms[:i], rooms[i+1:]...)
							break
						}
					}
				} else if exam.exam.Constraints.RoomConstraints.PlacesWithSocket {
					for i := 0; i < len(rooms); i++ {
						if rooms[i].PlacesWithSocket {
							room = rooms[i]
							rooms = append(rooms[:i], rooms[i+1:]...)
							break
						}
					}
				} else {
					room = rooms[0]
					rooms = rooms[1:]
				}
			} else {
				room = rooms[0]
				rooms = rooms[1:]
			}

			if room == nil {
				log.Error().Int("ancode", exam.exam.Exam.Ancode).
					Msg("no room found for exam")
				return fmt.Errorf("no room found for exam")
			}

			studentCountInRoom := room.Seats
			if studentCountInRoom > len(exam.normalRegs) {
				studentCountInRoom = len(exam.normalRegs)
			}

			studentsInRoom := exam.normalRegs[:studentCountInRoom]
			exam.normalRegs = exam.normalRegs[studentCountInRoom:]

			examRoom := model.RoomForExam{
				Ancode:       exam.exam.Exam.Ancode,
				Room:         room,
				SeatsPlanned: len(studentsInRoom),
				Duration:     exam.exam.Exam.ZpaExam.Duration,
				Handicap:     false,
				Students:     studentsInRoom,
			}

			exam.rooms = append(exam.rooms, &examRoom)
			examRooms = append(examRooms, &examRoom)

			exams = append(exams, exam)
		}
	}

	err := p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "rooms_for_exams"), examRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot save rooms for exams")
		return err
	}

	return nil
}
