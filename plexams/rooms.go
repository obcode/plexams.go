package plexams

import (
	"context"
	"fmt"

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
		slotsWithRooms = append(slotsWithRooms, &model.SlotWithRooms{
			DayNumber:  slot / 10,
			SlotNumber: slot % 10,
			Rooms:      rooms,
		})
	}

	return p.dbClient.SaveRooms(context.Background(), slotsWithRooms)
}

func (p *Plexams) Rooms(ctx context.Context) ([]*model.Room, error) {
	return p.dbClient.Rooms(ctx)
}

func (p *Plexams) RoomsForSlot(ctx context.Context, day int, time int) ([]*model.Room, error) {
	return p.dbClient.RoomsForSlot(ctx, day, time)
}

func (p *Plexams) AddRoomToExam(ctx context.Context, input model.RoomForExamInput) (bool, error) {
	// room allowed and enough seats in slot?
	// room, err := p.getRoom(ctx, input.RoomName, input.Day, input.Time, input.SeatsPlanned)
	// if err != nil {
	// 	log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
	// 		Msg("cannot get room")
	// 	return false, err
	// }

	return false, nil
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

	return nil, nil
}

func (p *Plexams) getPlannedRoom(ctx context.Context, roomName string, day, time, seatsNeeded int) ([]*model.Room, error) {
	// rooms, err := p.dbClient.RoomPlannedInSlot(ctx, roomName, day, time)
	// TODO: calculate the remaining seats? or in getRoom

	return nil, nil
}
