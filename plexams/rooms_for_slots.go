package plexams

import (
	"context"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrepareRoomsForSlots(approvedOnly bool) error {
	globalRooms, err := p.dbClient.Rooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}

	roomsWithRestrictedSlots, err := p.roomsWithRestrictedSlots()
	if err != nil {
		log.Error().Err(err).Msg("cannot get restricted slots for rooms")
		return err
	}

	slotsWithRoomNames := make(map[SlotNumber]set.Set[string])
	for _, slot := range p.semesterConfig.Slots {
		slotsWithRoomNames[SlotNumber{
			day:  slot.DayNumber,
			slot: slot.SlotNumber,
		}] = set.NewSet[string]()
	}

	for _, room := range globalRooms {
		restrictedSlots, ok := roomsWithRestrictedSlots[room.Name]
		if ok {
			for slot := range restrictedSlots.Iter() {
				slotsWithRoomNames[slot].Add(room.Name)
			}
		} else {
			// room is not restricted, so we can use all slots
			// for _, roomNames := range slotsWithRoomNames {
			// 	roomNames.Add(room.Name)
			// }
		}
	}

	roomsForSlots := make([]*model.RoomsForSlot, 0, len(slotsWithRoomNames))
	for slot, roomNames := range slotsWithRoomNames {
		roomNames := roomNames.ToSlice()
		sort.Strings(roomNames)
		roomsForSlots = append(roomsForSlots, &model.RoomsForSlot{
			Day:       slot.day,
			Slot:      slot.slot,
			RoomNames: roomNames,
		})
	}

	return p.dbClient.SaveRoomsForSlots(context.Background(), roomsForSlots)
}

func (p *Plexams) roomsWithRestrictedSlots() (map[string]set.Set[SlotNumber], error) {
	restrictedSlots := make(map[string]set.Set[SlotNumber])
	allSlots := set.NewSet[SlotNumber]()
	for _, slot := range p.semesterConfig.Slots {
		allSlots.Add(SlotNumber{
			day:  slot.DayNumber,
			slot: slot.SlotNumber,
		})
	}

	// EXaHM rooms
	restrictedSlotsForEXaHMRooms, err := p.restrictedSlotsForEXaHMRooms()
	if err != nil {
		log.Error().Err(err).Msg("cannot get allowed slots for EXaHM rooms")
		return nil, err
	}
	for roomName, slots := range restrictedSlotsForEXaHMRooms {
		restrictedSlots[roomName] = slots
	}

	// TODO: Add other room types with restricted slots

	return restrictedSlots, nil
}

func (p *Plexams) restrictedSlotsForEXaHMRooms() (map[string]set.Set[SlotNumber], error) {
	restrictedSlots := make(map[string]set.Set[SlotNumber])
	// EXaHM rooms
	bookedEntries, err := p.ExahmRoomsFromBooked()
	if err != nil {
		log.Error().Err(err).Msg("cannot get exahm rooms from booked")
		return nil, err
	}

	for _, slot := range p.semesterConfig.Slots {
		for _, entry := range bookedEntries {
			if entry.From.Before(slot.Starttime.Local()) &&
				entry.Until.After(slot.Starttime.Local().Add(89*time.Minute)) {
				for _, roomName := range entry.Rooms {
					if _, ok := restrictedSlots[roomName]; !ok {
						restrictedSlots[roomName] = set.NewSet[SlotNumber]()
					}
					restrictedSlots[roomName].Add(SlotNumber{
						day:  slot.DayNumber,
						slot: slot.SlotNumber,
					})
				}
			}
		}
	}

	return restrictedSlots, nil
}
