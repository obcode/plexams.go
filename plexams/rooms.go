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

type SlotNumber struct {
	day, slot int
}

/*
roomConstraints:
  booked:
    - date: 2024-01-22
      from: "14:00"
      until: "17:00"
      rooms:
        - T3.015
        - T3.016
        - T3.017
        - T3.023
*/

type BookedEntry struct {
	From  time.Time
	Until time.Time
	Rooms []string
}

func (p *Plexams) ExahmRoomsFromBooked() ([]BookedEntry, error) {
	bookedInfo := viper.Get("roomconstraints.booked")

	bookedInfoSlice, ok := bookedInfo.([]interface{})
	if !ok {
		log.Error().Interface("booked info", bookedInfo).Msg("cannot convert booked info to slice")
		return nil, fmt.Errorf("cannot convert booked info to slice")
	}

	entries := make([]BookedEntry, 0, len(bookedInfoSlice))
	for _, bookedEntry := range bookedInfoSlice {
		fromUntil, err := fromUntil(bookedEntry)
		if err != nil {
			log.Error().Err(err).Interface("entry", bookedEntry).Msg("cannot convert entry to time")
			return nil, err
		}

		entry, ok := bookedEntry.(map[string]interface{})
		if !ok {
			log.Error().Interface("booked entry", bookedEntry).Msg("cannot convert booked entry to map")
			return nil, fmt.Errorf("cannot convert booked entry to map")
		}

		rawRooms, ok := entry["rooms"].([]interface{})
		if !ok {
			log.Error().Interface("rooms entry", entry["rooms"]).Msg("cannot convert rooms entry to []string")
			return nil, fmt.Errorf("cannot convert rooms entry to []string")
		}

		rooms := make([]string, 0, len(rawRooms))
		for _, rawRoom := range rawRooms {
			room, ok := rawRoom.(string)
			if !ok {
				log.Error().Interface("room entry", rawRoom).Msg("cannot convert room entry to string")
				return nil, fmt.Errorf("cannot convert room entry to string")
			}
			rooms = append(rooms, room)
		}

		entries = append(entries, BookedEntry{
			From:  fromUntil.From,
			Until: fromUntil.Until,
			Rooms: rooms,
		})

	}

	return entries, nil
}

func (p *Plexams) SlotsWithRoomsFromBookedEntries(bookedEntries []BookedEntry) (map[SlotNumber][]*model.Room, error) {
	globalRooms, err := p.dbClient.Rooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return nil, err
	}

	globalRoomsMap := make(map[string]*model.Room)
	for _, room := range globalRooms {
		globalRoomsMap[room.Name] = room
	}

	slotsWithRooms := make(map[SlotNumber][]*model.Room)

	for _, slot := range p.semesterConfig.Slots {
		for _, entry := range bookedEntries {
			if entry.From.Before(slot.Starttime.Local()) && entry.Until.After(slot.Starttime.Local().Add(89*time.Minute)) {
				rooms := make([]*model.Room, 0, len(entry.Rooms))
				for _, roomName := range entry.Rooms {
					room, ok := globalRoomsMap[roomName]
					if !ok {
						log.Error().Str("room name", roomName).Msg("room not found")
						return nil, fmt.Errorf("room %s not found", roomName)
					}
					rooms = append(rooms, room)
				}
				slotsWithRooms[SlotNumber{slot.DayNumber, slot.SlotNumber}] = rooms
			}
		}
	}

	return slotsWithRooms, nil
}

func (p *Plexams) PrepareRoomsForSemester() error {
	globalRooms, err := p.dbClient.Rooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}

	roomsForSlots := make(map[SlotNumber][]*model.Room)
	for _, room := range globalRooms {
		if room.Name == "No Room" || room.Exahm {
			continue
		}
		roomConstraints := viper.Get(fmt.Sprintf("roomConstraints.%s", room.Name))
		if roomConstraints == nil {

			if room.NeedsRequest {
				fmt.Printf("%s: no constraints found, but room needs request, ignoring room\n", room.Name)
				continue
			}

			fmt.Printf("%s: no constraints found\n", room.Name)

			for _, slot := range p.semesterConfig.Slots {
				slotNumber := SlotNumber{slot.DayNumber, slot.SlotNumber}
				slotEntry, ok := roomsForSlots[slotNumber]
				if !ok {
					slotEntry = []*model.Room{room}
				} else {
					slotEntry = append(slotEntry, room)
				}
				roomsForSlots[slotNumber] = slotEntry
			}
		} else {
			//   R1.046:
			//     reservations:
			//       - date: 2024-01-24
			//         from: 10:15
			//         until: 12:15
			//       - date: 2024-01-24
			//         from: 14:15
			//         until: 16:15
			reservations := viper.Get(fmt.Sprintf("roomConstraints.%s.reservations", room.Name))
			if reservations != nil {
				fmt.Printf("%s: reservations found\n", room.Name)
				reservationsSlice, ok := reservations.([]interface{})
				if !ok {
					log.Error().Interface("reservations", reservations).Msg("cannot convert reservations to slice")
					return fmt.Errorf("cannot convert reservations to slice")
				}
				reservedSlots, err := p.reservations2Slots(reservationsSlice)
				if err != nil {
					log.Error().Err(err).Msg("cannot convert reservations to slots")
					return err
				}
				for _, slot := range reservedSlots.ToSlice() {
					slotNumber := SlotNumber{slot.day, slot.slot}
					slotEntry, ok := roomsForSlots[slotNumber]
					if !ok {
						slotEntry = []*model.Room{room}
					} else {
						slotEntry = append(slotEntry, room)
					}
					roomsForSlots[slotNumber] = slotEntry
				}
			}
		}
	}

	bookedEntries, err := p.ExahmRoomsFromBooked()
	if err != nil {
		log.Error().Err(err).Msg("cannot get exahm rooms from booked")
		return err
	}
	bookedRoomsMap, err := p.SlotsWithRoomsFromBookedEntries(bookedEntries)
	if err != nil {
		log.Error().Err(err).Msg("cannot get booked rooms map from booked entries")
		return err
	}

	slotsWithRooms := make([]*model.SlotWithRooms, 0, len(roomsForSlots))
	for slot, rooms := range roomsForSlots {
		normalRooms, _, labRooms, ntaRooms := splitRooms(rooms)
		exahmRooms := bookedRoomsMap[slot]
		slotsWithRooms = append(slotsWithRooms, &model.SlotWithRooms{
			DayNumber:   slot.day,
			SlotNumber:  slot.slot,
			NormalRooms: normalRooms,
			ExahmRooms:  exahmRooms,
			LabRooms:    labRooms,
			NtaRooms:    ntaRooms,
		})
	}

	return p.dbClient.SaveRooms(context.Background(), slotsWithRooms)
}

type TimeRange struct {
	From  time.Time
	Until time.Time
}

func (p *Plexams) GetReservations() (map[string][]TimeRange, error) {
	ctx := context.Background()
	rooms, err := p.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get rooms")
	}

	reservations := make(map[string][]TimeRange)

	for _, room := range rooms {
		if viper.IsSet(fmt.Sprintf("roomConstraints.%s.reservations", room.Name)) {
			log.Debug().Str("room", room.Name).Msg("found reservations for room")
			reservationsForRoom := viper.Get(fmt.Sprintf("roomConstraints.%s.reservations", room.Name))
			reservationsSlice, ok := reservationsForRoom.([]interface{})
			if !ok {
				log.Error().Interface("reservations", reservations).Msg("cannot convert reservations to slice")
				return nil, fmt.Errorf("cannot convert reservations to slice")
			}

			reservations[room.Name] = make([]TimeRange, 0, len(reservationsSlice))

			for _, reservationEntry := range reservationsSlice {
				fromUntil, err := fromUntil(reservationEntry)
				if err != nil {
					log.Error().Err(err).Interface("reservation", reservationsSlice).Msg("cannot convert reservation to time")
					return nil, err
				}
				reservations[room.Name] = append(reservations[room.Name], *fromUntil)
			}
		}
	}

	return reservations, nil
}

func (p *Plexams) reservations2Slots(reservations []interface{}) (set.Set[SlotNumber], error) {
	slots := set.NewSet[SlotNumber]()
	for _, reservation := range reservations {
		fromUntil, err := fromUntil(reservation)
		if err != nil {
			log.Error().Err(err).Interface("reservation", reservation).Msg("cannot convert reservation to time")
			return nil, err
		}

		fmt.Printf("    From: %v Until: %v\n", fromUntil.From, fromUntil.Until)

		for _, slot := range p.semesterConfig.Slots {
			if (fromUntil.From.Before(slot.Starttime.Local()) || fromUntil.From.Equal(slot.Starttime.Local())) && fromUntil.Until.After(slot.Starttime.Local().Add(89*time.Minute)) {
				fmt.Printf("        ---> add (%d, %d)\n", slot.DayNumber, slot.SlotNumber)
				slots.Add(SlotNumber{slot.DayNumber, slot.SlotNumber})
			}
		}
	}
	return slots, nil
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

	return &TimeRange{
		From:  from,
		Until: until,
	}, nil
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

func (p *Plexams) ChangeRoom(ctx context.Context, ancode int, oldRoomName, newRoomName string) (bool, error) {
	roomsForAncode, err := p.dbClient.RoomsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting rooms for ancode")
		return false, err
	}

	var oldRoom *model.Room
	for _, room := range roomsForAncode {
		if room.RoomName == oldRoomName {
			log.Debug().Msg("old room found")
			oldRoom = p.GetRoomInfo(room.RoomName)
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

func (p *Plexams) PlannedRoomsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedRoom, error) {
	rooms, err := p.dbClient.PlannedRoomsInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get exams in slot")
	}

	return rooms, nil
}

// func enhancePlannedRooms(plannedRooms []*model.PlannedRoom) []*model.EnhancedPlannedRoom {
// 	enhancedPlannedRooms := make([]*model.EnhancedPlannedRoom, 0, len(plannedRooms))
// 	for _, room := range plannedRooms {
// 		enhancedPlannedRooms = append(enhancedPlannedRooms, &model.EnhancedPlannedRoom{
// 			Day:               room.Day,
// 			Slot:              room.Ancode,
// 			RoomName:          room.RoomName,
// 			Ancode:            room.Ancode,
// 			Duration:          room.Duration,
// 			Handicap:          room.Handicap,
// 			HandicapRoomAlone: room.HandicapRoomAlone,
// 			Reserve:           room.Reserve,
// 			StudentsInRoom:    room.StudentsInRoom,
// 			NtaMtknr:          room.NtaMtknr,
// 		})
// 	}
// 	return enhancedPlannedRooms
// }

func (p *Plexams) PlannedRoomNamesInSlot(ctx context.Context, day int, time int) ([]string, error) {
	return p.dbClient.PlannedRoomNamesInSlot(ctx, day, time)
}

func (p *Plexams) PlannedRooms(ctx context.Context) ([]*model.PlannedRoom, error) {
	return p.dbClient.PlannedRooms(ctx)
}

func (p *Plexams) RoomFromName(ctx context.Context, roomName string) (*model.Room, error) {
	return p.dbClient.RoomFromName(ctx, roomName)
}
