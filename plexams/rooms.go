package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) PrepareRooms() error {
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
	room, err := p.getRoom(ctx, input.RoomName, input.Day, input.Time, input.SeatsPlanned)
	if err != nil {
		log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
			Msg("cannot get room")
		return false, err
	}

	err = p.dbClient.AddRoomToExam(ctx, &model.RoomForExam{
		Ancode:       input.Ancode,
		Room:         room,
		SeatsPlanned: input.SeatsPlanned,
		Duration:     input.Duration,
		Handicap:     input.Handicap,
		Mktnrs:       input.Mktnrs,
	})
	if err != nil {
		log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
			Msg("cannot save room to db")
		return false, err
	}

	return true, nil
}

func (p *Plexams) getRoom(ctx context.Context, roomName string, day, time, seatsNeeded int) (*model.Room, error) {
	roomsForSlot, err := p.RoomsForSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get rooms for slot")
		return nil, err
	}
	var room *model.Room
	for _, roomForSlot := range roomsForSlot {
		if roomName == roomForSlot.Name {
			room = roomForSlot
			break
		}
	}
	if room == nil {
		return nil, fmt.Errorf("room %s for slot (%d,%d) not allowed", roomName, day, time)
	}

	roomAlreadyPlanned, err := p.dbClient.RoomPlannedInSlot(ctx, roomName, day, time)
	if err != nil {
		log.Error().Err(err).Str("room name", roomName).Int("day", day).Int("time", time).
			Msg("cannot get plannedrooms for slot")
		return nil, err
	}

	seatsUsedAlready := 0
	for _, roomInUse := range roomAlreadyPlanned {
		seatsUsedAlready += roomInUse.SeatsPlanned
	}

	if room.Seats-seatsUsedAlready < seatsNeeded {
		log.Debug().Str("room name", roomName).Int("day", day).Int("time", time).
			Msg("not enough seats left")
		return nil, fmt.Errorf("not enough seats left")
	}

	return room, nil
}

func (p *Plexams) PrepareRoomForExams() error {
	type exam struct {
		ancode int
		regs   int
		ntas   []*model.NTAWithRegs
	}

	insertSorted := func(exams []exam, examToInsert exam) []exam {
		i := 0
		for _, exam := range exams {
			if exam.regs < examToInsert.regs {
				break
			}
			i++
		}
		if i >= len(exams) {
			return append(exams, examToInsert)
		}
		return append(append(exams[:i], examToInsert), exams[i:]...)
	}

	ctx := context.Background()
	for _, slot := range p.semesterConfig.Slots {
		// get exams
		examsInPlan, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to find exams in slot")
			return err
		}

		exams := make([]exam, 0, len(examsInPlan))
		for _, examInPlan := range examsInPlan {
			ntas := examInPlan.Nta
			regs := 0
			for _, registrations := range examInPlan.Exam.StudentRegs {
				regs += len(registrations.StudentRegs)
			}
			regs -= len(ntas)

			exams = insertSorted(exams, exam{
				ancode: examInPlan.Exam.Ancode,
				regs:   regs,
				ntas:   ntas,
			})
		}

		// get rooms
		// rooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		// if err != nil {
		// 	log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
		// 		Msg("error while trying to get rooms for slot")
		// 	return err
		// }

		// for {
		// 	// find exam with most seats needed

		// 	// find biggest room
		// }

	}

	return nil
}
