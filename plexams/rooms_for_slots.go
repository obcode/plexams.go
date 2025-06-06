package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) PrepareRoomsForSlots(approvedOnly bool) error {
	ctx := context.Background()
	globalRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}

	roomsWithRestrictedSlots, err := p.roomsWithRestrictedSlots(globalRooms)
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
			for _, roomNames := range slotsWithRoomNames {
				roomNames.Add(room.Name)
			}
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

func (p *Plexams) roomsWithRestrictedSlots(globalRooms []*model.Room) (map[string]set.Set[SlotNumber], error) {
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
	restrictedSlotsForOtherRooms, err := p.restrictedSlotsForOtherRooms(globalRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot get allowed slots for other rooms")
		return nil, err
	}

	for roomName, slots := range restrictedSlotsForOtherRooms {
		restrictedSlots[roomName] = slots
	}

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

func (p *Plexams) restrictedSlotsForOtherRooms(globalRooms []*model.Room) (map[string]set.Set[SlotNumber], error) {
	allSlots := p.semesterConfig.Slots

	restrictedSlots := make(map[string]set.Set[SlotNumber])
	for _, room := range globalRooms {
		roomConstraints := viper.Get(fmt.Sprintf("roomConstraints.%s", room.Name))
		if roomConstraints != nil {
			restrictedSlots[room.Name] = set.NewSet[SlotNumber]()
			reservations := viper.Get(fmt.Sprintf("roomConstraints.%s.reservations", room.Name))
			if reservations != nil {
				reservationsSlice, ok := reservations.([]interface{})
				if !ok {
					log.Error().Interface("reservations", reservations).Msg("cannot convert reservations to slice")
					return nil, fmt.Errorf("cannot convert reservations to slice")
				}
				reservedSlots, err := p.reservations2Slots(reservationsSlice, false)
				if err != nil {
					log.Error().Err(err).Msg("cannot convert reservations to slots")
					return nil, err
				}
				for slot := range reservedSlots.Iter() {
					restrictedSlots[room.Name].Add(slot)
				}
			}
			notAvailable := viper.Get(fmt.Sprintf("roomConstraints.%s.notAvailable", room.Name))
			if notAvailable != nil {
				notAvailableSlice, ok := notAvailable.([]interface{})
				if !ok {
					log.Error().Interface("notAvailable", notAvailable).Msg("cannot convert notAvailable to slice")
					return nil, fmt.Errorf("cannot convert notAvailable to slice")
				}
				for _, notAvailableEntry := range notAvailableSlice {
					rawDate, ok := notAvailableEntry.(time.Time)
					if !ok {
						log.Error().Interface("notAvailableEntry", notAvailableEntry).Msg("cannot convert notAvailable entry to time")
						return nil, fmt.Errorf("cannot convert notAvailable entry to time")
					}
					fmt.Printf("rawDate: %s\n", rawDate.Local().Format("2006-01-02 15:04"))
					for _, slot := range allSlots {
						// TODO: stimmt nicht!
						if slot.Starttime.Local().Year() != rawDate.Year() ||
							slot.Starttime.Local().Month() != rawDate.Month() ||
							slot.Starttime.Local().Day() != rawDate.Day() {
							if _, ok := restrictedSlots[room.Name]; !ok {
								restrictedSlots[room.Name] = set.NewSet[SlotNumber]()
							}
							restrictedSlots[room.Name].Add(SlotNumber{
								day:  slot.DayNumber,
								slot: slot.SlotNumber,
							})
						}
					}
				}
			}
		}
	}

	return restrictedSlots, nil
}

func fromUntil(dateEntry interface{}) (fromUntil *TimeRange, err error) {
	entry, ok := dateEntry.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("cannot convert date entry to map")
		log.Error().Interface("date entry", dateEntry).Msg("cannot convert date entry to map")
		return nil, err
	}

	rawDate, ok := entry["date"].(time.Time)
	if !ok {
		err = fmt.Errorf("cannot convert date entry to string")
		log.Error().Interface("date entry", entry["date"]).Msg("cannot convert date entry to string")
		return nil, err
	}
	rawFrom, ok := entry["from"].(string)
	if !ok {
		err = fmt.Errorf("cannot convert from entry to string")
		log.Error().Interface("date entry", entry["from"]).Msg("cannot convert from entry to string")
		return nil, err
	}
	rawUntil, ok := entry["until"].(string)
	if !ok {
		err = fmt.Errorf("cannot convert until entry to string")
		log.Error().Interface("date entry", entry["until"]).Msg("cannot convert until entry to string")
		return nil, err
	}

	from, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", rawDate.Format("2006-01-02"), rawFrom), time.Local)
	if err != nil {
		log.Error().Err(err).Interface("date", rawDate).Str("time", rawFrom).Msg("cannot parse to time")
		return nil, err
	}
	until, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", rawDate.Format("2006-01-02"), rawUntil), time.Local)
	if err != nil {
		log.Error().Err(err).Interface("date", rawDate).Str("time", rawFrom).Msg("cannot parse to time")
		return nil, err
	}

	dayNumber := -1
	slotNumber := -1
	slot, ok := entry["slot"].([]interface{})
	if ok {
		dayNumber = slot[0].(int)
		slotNumber = slot[1].(int)
	}
	approved := entry["approved"].(bool)

	return &TimeRange{
		From:       from,
		Until:      until,
		DayNumber:  dayNumber,
		SlotNumber: slotNumber,
		Approved:   approved,
	}, nil
}
