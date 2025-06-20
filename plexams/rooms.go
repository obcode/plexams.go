package plexams

import (
	"context"
	"fmt"
	"time"

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
	From     time.Time
	Until    time.Time
	Rooms    []string
	Approved bool
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

		approved := entry["approved"].(bool)

		entries = append(entries, BookedEntry{
			From:     fromUntil.From,
			Until:    fromUntil.Until,
			Rooms:    rooms,
			Approved: approved,
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

// func (p *Plexams) PrepareRoomsForSemester(approvedOnly bool) error {
// 	globalRooms, err := p.dbClient.Rooms(context.Background())
// 	if err != nil {
// 		log.Error().Err(err).Msg("cannot get global rooms")
// 		return err
// 	}

// 	roomsForSlots := make(map[SlotNumber][]*model.Room)
// 	for _, room := range globalRooms {
// 		if room.Name == "No Room" || room.Exahm {
// 			continue
// 		}
// 		roomConstraints := viper.Get(fmt.Sprintf("roomConstraints.%s", room.Name))
// 		if roomConstraints == nil {

// 			if room.NeedsRequest {
// 				fmt.Println(aurora.Sprintf(aurora.Red("%s: no constraints found, but room needs request, ignoring room"),
// 					aurora.Cyan(room.Name)))
// 				continue
// 			}

// 			fmt.Println(aurora.Sprintf(aurora.Green("%s: no constraints found"), aurora.Cyan(room.Name)))

// 			for _, slot := range p.semesterConfig.Slots {
// 				slotNumber := SlotNumber{slot.DayNumber, slot.SlotNumber}
// 				slotEntry, ok := roomsForSlots[slotNumber]
// 				if !ok {
// 					slotEntry = []*model.Room{room}
// 				} else {
// 					slotEntry = append(slotEntry, room)
// 				}
// 				roomsForSlots[slotNumber] = slotEntry
// 			}
// 		} else {
// 			//   R1.046:
// 			//     reservations:
// 			//       - slot: [1,3]
// 			//         date: 2024-01-24
// 			//         from: 10:15
// 			//         until: 12:15
// 			//       - slot: [1, 5]
// 			//         date: 2024-01-24
// 			//         from: 14:15
// 			//         until: 16:15
// 			reservations := viper.Get(fmt.Sprintf("roomConstraints.%s.reservations", room.Name))
// 			if reservations != nil {
// 				fmt.Println(aurora.Sprintf(aurora.Green("%s: reservations found"), aurora.Cyan(room.Name)))

// 				reservationsSlice, ok := reservations.([]interface{})
// 				if !ok {
// 					log.Error().Interface("reservations", reservations).Msg("cannot convert reservations to slice")
// 					return fmt.Errorf("cannot convert reservations to slice")
// 				}
// 				reservedSlots, err := p.reservations2Slots(reservationsSlice, room.Name, approvedOnly)
// 				if err != nil {
// 					log.Error().Err(err).Msg("cannot convert reservations to slots")
// 					return err
// 				}
// 				for _, slot := range reservedSlots.ToSlice() {
// 					slotNumber := SlotNumber{slot.day, slot.slot}
// 					slotEntry, ok := roomsForSlots[slotNumber]
// 					if !ok {
// 						slotEntry = []*model.Room{room}
// 					} else {
// 						slotEntry = append(slotEntry, room)
// 					}
// 					roomsForSlots[slotNumber] = slotEntry
// 				}
// 			}
// 			// notAllowedDays := viper.Get(fmt.Sprintf("roomConstraints.%s.notAllowedDays", room.Name))
// 			// if notAllowedDays != nil {

// 			// }
// 		}
// 	}

// 	bookedEntries, err := p.ExahmRoomsFromBooked()
// 	if err != nil {
// 		log.Error().Err(err).Msg("cannot get exahm rooms from booked")
// 		return err
// 	}
// 	bookedRoomsMap, err := p.SlotsWithRoomsFromBookedEntries(bookedEntries)
// 	if err != nil {
// 		log.Error().Err(err).Msg("cannot get booked rooms map from booked entries")
// 		return err
// 	}

// 	slotsWithRooms := make([]*model.SlotWithRooms, 0, len(roomsForSlots))
// 	for slot, rooms := range roomsForSlots {
// 		normalRooms, _, labRooms, ntaRooms := splitRooms(rooms)
// 		exahmRooms := bookedRoomsMap[slot]
// 		slotsWithRooms = append(slotsWithRooms, &model.SlotWithRooms{
// 			DayNumber:   slot.day,
// 			SlotNumber:  slot.slot,
// 			NormalRooms: normalRooms,
// 			ExahmRooms:  exahmRooms,
// 			LabRooms:    labRooms,
// 			NtaRooms:    ntaRooms,
// 		})
// 	}

// 	return p.dbClient.SaveRooms(context.Background(), slotsWithRooms)
// }

type TimeRange struct {
	From       time.Time
	Until      time.Time
	DayNumber  int
	SlotNumber int
	Approved   bool
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

// func splitRooms(rooms []*model.Room) ([]*model.Room, []*model.Room, []*model.Room, []*model.Room) {
// 	normalRooms := make([]*model.Room, 0)
// 	exahmRooms := make([]*model.Room, 0)
// 	labRooms := make([]*model.Room, 0)
// 	ntaRooms := make([]*model.Room, 0)
// 	for _, room := range rooms {
// 		if room.Handicap {
// 			ntaRooms = append(ntaRooms, room)
// 		} else if room.Exahm {
// 			exahmRooms = append(exahmRooms, room)
// 		} else if room.Lab {
// 			labRooms = append(labRooms, room)
// 		} else {
// 			normalRooms = append(normalRooms, room)
// 		}
// 	}
// 	sort.Slice(normalRooms, func(i, j int) bool { return normalRooms[i].Seats > normalRooms[j].Seats })
// 	sort.Slice(exahmRooms, func(i, j int) bool { return exahmRooms[i].Seats > exahmRooms[j].Seats })
// 	sort.Slice(labRooms, func(i, j int) bool { return labRooms[i].Seats > labRooms[j].Seats })
// 	sort.Slice(ntaRooms, func(i, j int) bool { return ntaRooms[i].Seats < ntaRooms[j].Seats })
// 	return normalRooms, exahmRooms, labRooms, ntaRooms
// }

func (p *Plexams) Rooms(ctx context.Context) ([]*model.Room, error) {
	return p.dbClient.Rooms(ctx)
}

func (p *Plexams) RoomsForSlots(ctx context.Context) ([]*model.RoomsForSlot, error) {
	return p.dbClient.RoomsForSlots(ctx)
}

func (p *Plexams) RoomsForSlot(ctx context.Context, day int, time int) (*model.RoomsForSlot, error) {
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

func (p *Plexams) PreAddRoomToExam(ctx context.Context, ancode int, roomName string, mtknr *string, reserve bool) (bool, error) {
	room, err := p.dbClient.RoomByName(ctx, roomName)
	if err != nil {
		log.Error().Err(err).Str("room", roomName).Msg("cannot get room from name")
		return false, err
	}

	if room == nil {
		log.Error().Str("room", roomName).Msg("room not found")
		return false, fmt.Errorf("room %s not found", roomName)
	}

	if mtknr != nil {
		student, err := p.StudentByMtknr(ctx, *mtknr, nil)
		if err != nil {
			log.Error().Err(err).Str("mtknr", *mtknr).Msg("cannot get student by mtknr")
			return false, err
		}
		if student == nil {
			log.Error().Str("mtknr", *mtknr).Msg("student not found")
			return false, fmt.Errorf("student with mtknr %s not found", *mtknr)
		}
		reserve = false // room for one student is NTA and never reserve
	}

	return p.dbClient.AddPrePlannedRoomToExam(ctx, &model.PrePlannedRoom{
		Ancode:   ancode,
		RoomName: roomName,
		Mtknr:    mtknr,
		Reserve:  reserve,
	})
}

func (p *Plexams) ChangeRoom(ctx context.Context, ancode int, oldRoomName, newRoomName string) (bool, error) {
	return false, fmt.Errorf("ChangeRoom is not implemented yet")
	// 	roomsForAncode, err := p.dbClient.RoomsForAncode(ctx, ancode)
	// 	if err != nil {
	// 		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting rooms for ancode")
	// 		return false, err
	// 	}

	// 	var oldRoom *model.Room
	// 	for _, room := range roomsForAncode {
	// 		if room.RoomName == oldRoomName {
	// 			log.Debug().Msg("old room found")
	// 			oldRoom = p.GetRoomInfo(room.RoomName)
	// 		}
	// 	}
	// 	if oldRoom == nil {
	// 		log.Error().Msg("old room not found")
	// 		return false, fmt.Errorf("old room %s for ancode %d not found", oldRoomName, ancode)
	// 	}

	// 	slot, err := p.SlotForAncode(ctx, ancode)
	// 	if err != nil || slot == nil {
	// 		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting slot for ancode")
	// 		return false, err
	// 	}

	// 	roomsForSlot, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
	// 	if err != nil || roomsForSlot == nil {
	// 		log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
	// 			Msg("error while getting rooms for slot")
	// 		return false, err
	// 	}

	// 	var newRoom *model.Room

	// 	if oldRoom.Exahm {
	// 		for _, roomForSlot := range roomsForSlot.ExahmRooms {
	// 			if roomForSlot.Name == newRoomName {
	// 				newRoom = roomForSlot
	// 			}
	// 		}
	// 	} else if oldRoom.Lab {
	// 		for _, roomForSlot := range roomsForSlot.LabRooms {
	// 			if roomForSlot.Name == newRoomName {
	// 				newRoom = roomForSlot
	// 			}
	// 		}
	// 	} else {
	// 		for _, roomForSlot := range roomsForSlot.NormalRooms {
	// 			if roomForSlot.Name == newRoomName {
	// 				newRoom = roomForSlot
	// 			}
	// 		}
	// 	}

	// 	if newRoom == nil {
	// 		log.Error().Msg("old room not found")
	// 		return false, fmt.Errorf("new room %s for ancode %d not found", newRoomName, ancode)
	// 	}

	// return p.dbClient.ChangeRoom(ctx, ancode, oldRoom, newRoom)
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

func (p *Plexams) PlannedRoomForStudent(ctx context.Context, ancode int, mtknr string) (*model.PlannedRoom, error) {
	plannedRoomsForExam, err := p.dbClient.PlannedRoomsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get planned rooms for ancode")
		return nil, err
	}
	for _, room := range plannedRoomsForExam {
		for _, student := range room.StudentsInRoom {
			if student == mtknr {
				return room, nil
			}
		}
	}

	// err = fmt.Errorf("student %s not found in planned rooms for ancode %d", mtknr, ancode)
	// log.Error().Err(err).Int("ancode", ancode).Str("mtknr", mtknr).Msg("student not found in planned rooms")
	return nil, nil
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

func (p *Plexams) RoomByName(ctx context.Context, roomName string) (*model.Room, error) {
	return p.dbClient.RoomByName(ctx, roomName)
}
