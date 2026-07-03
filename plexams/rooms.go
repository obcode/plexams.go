package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type SlotNumber struct {
	day, slot int
}

// AnnyRoomBooking is one or more rooms booked in Anny for a time window, with the
// booking's approval status. It is derived from the stored Anny bookings by
// ExahmRoomsFromAnnyBookings (adjacent/overlapping bookings of the same room merged).
type AnnyRoomBooking struct {
	From     time.Time
	Until    time.Time
	Rooms    []string
	Approved bool
}

func (p *Plexams) ExahmRoomsFromAnnyBookings(ctx context.Context) ([]AnnyRoomBooking, error) {
	dbBookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get anny bookings from db: %w", err)
	}
	if len(dbBookings) == 0 {
		return nil, nil
	}

	// Only consider rooms marked as Anny rooms in the rooms master data (RequestWith==ANNY)
	allowedRooms, err := p.anny.RoomNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get anny rooms: %w", err)
	}
	// only OUR bookings count as available capacity — other faculties' bookings in the
	// same rooms are in the DB for information only.
	names := p.anny.PersonalizationNames(ctx)

	entries := make([]AnnyRoomBooking, 0, len(dbBookings))
	for _, booking := range dbBookings {
		if booking.Room == "" {
			continue
		}
		if !anny.MatchesAnyPersonalization(booking.PersonalizationName, names) {
			continue
		}
		normalizedRoom := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(booking.Room), " ", ""))
		if _, ok := allowedRooms[normalizedRoom]; !ok {
			continue
		}
		entries = append(entries, AnnyRoomBooking{
			From:     booking.StartDate,
			Until:    booking.EndDate,
			Rooms:    []string{booking.Room},
			Approved: anny.IsApprovedStatus(booking.Status),
		})
	}

	return mergeAnnyRoomBookings(entries), nil
}

func mergeAnnyRoomBookings(entries []AnnyRoomBooking) []AnnyRoomBooking {
	if len(entries) < 2 {
		return entries
	}

	sortedEntries := make([]AnnyRoomBooking, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Rooms) != 1 {
			sortedEntries = append(sortedEntries, entry)
			continue
		}
		sortedEntries = append(sortedEntries, entry)
	}

	sort.Slice(sortedEntries, func(i, j int) bool {
		roomI := ""
		if len(sortedEntries[i].Rooms) > 0 {
			roomI = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(sortedEntries[i].Rooms[0]), " ", ""))
		}
		roomJ := ""
		if len(sortedEntries[j].Rooms) > 0 {
			roomJ = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(sortedEntries[j].Rooms[0]), " ", ""))
		}

		if roomI != roomJ {
			return roomI < roomJ
		}
		if sortedEntries[i].Approved != sortedEntries[j].Approved {
			return sortedEntries[i].Approved && !sortedEntries[j].Approved
		}
		if !sortedEntries[i].From.Equal(sortedEntries[j].From) {
			return sortedEntries[i].From.Before(sortedEntries[j].From)
		}
		return sortedEntries[i].Until.Before(sortedEntries[j].Until)
	})

	merged := make([]AnnyRoomBooking, 0, len(sortedEntries))
	for _, current := range sortedEntries {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}

		last := &merged[len(merged)-1]
		if len(last.Rooms) != 1 || len(current.Rooms) != 1 {
			merged = append(merged, current)
			continue
		}

		lastRoom := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(last.Rooms[0]), " ", ""))
		currentRoom := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(current.Rooms[0]), " ", ""))

		// Merge adjacent or overlapping bookings for the same room and approval status.
		if lastRoom == currentRoom &&
			last.Approved == current.Approved &&
			(current.From.Before(last.Until) || current.From.Equal(last.Until)) {
			if current.Until.After(last.Until) {
				last.Until = current.Until
			}
			continue
		}

		merged = append(merged, current)
	}

	return merged
}

type TimeRange struct {
	From       time.Time
	Until      time.Time
	DayNumber  int
	SlotNumber int
	Approved   bool
}

// GetReservations returns the active building-management room requests as time
// ranges per room, read from the DB (collection room_requests). It feeds room
// planning (restrictedSlotsForOtherRooms) and the needs-request validation;
// inactive requests are excluded so they no longer count for planning.
func (p *Plexams) GetReservations() (map[string][]TimeRange, error) {
	ctx := context.Background()
	requests, err := p.dbClient.RoomRequests(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get room requests")
		return nil, err
	}

	reservations := make(map[string][]TimeRange)
	for _, r := range requests {
		if !r.Active {
			continue
		}
		reservations[r.Room] = append(reservations[r.Room], TimeRange{
			From:       r.From,
			Until:      r.Until,
			DayNumber:  r.Day,
			SlotNumber: r.Slot,
			Approved:   r.Approved,
		})
	}
	return reservations, nil
}

// reservationsFromConfig reads the room reservations from the semester config
// (roomConstraints.<room>.reservations). Used only for the one-time migration
// into the DB.
func (p *Plexams) reservationsFromConfig() (map[string][]TimeRange, error) {
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
	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	for _, room := range rooms {
		normalizeRoomRequestWith(room)
	}
	return rooms, nil
}

// requestWithForRoom derives how a room must be requested from its needsRequest
// flag and its name: T-building rooms go via Anny, all other request-rooms via
// the building management.
func requestWithForRoom(room *model.Room) model.RoomRequestType {
	if !room.NeedsRequest {
		return model.RoomRequestTypeNone
	}
	if strings.HasPrefix(room.Name, "T") {
		return model.RoomRequestTypeAnny
	}
	return model.RoomRequestTypeManagement
}

// normalizeRoomRequestWith fills requestWith for rooms whose document predates
// the field (so reads are robust before the one-time backfill ran).
func normalizeRoomRequestWith(room *model.Room) {
	if room == nil {
		return
	}
	if room.RequestWith == "" {
		room.RequestWith = requestWithForRoom(room)
	}
}

// SetRoomActive activates/deactivates a room (key: name). A deactivated room is
// not used when computing the rooms available for planning. Errors if the room
// does not exist.
func (p *Plexams) SetRoomActive(ctx context.Context, name string, active bool) (*model.Room, error) {
	return p.dbClient.SetRoomDeactivated(ctx, name, !active)
}

func roomInputToRoom(input model.RoomInput) *model.Room {
	return &model.Room{
		Name:             input.Name,
		Seats:            input.Seats,
		Handicap:         input.Handicap,
		Lab:              input.Lab,
		PlacesWithSocket: input.PlacesWithSocket,
		RequestWith:      input.RequestWith,
		RequestPriority:  input.RequestPriority,
		NeedsRequest:     input.RequestWith != model.RoomRequestTypeNone,
		Exahm:            input.Exahm,
		Seb:              input.Seb,
		SebSeats:         input.SebSeats,
		HmebSeats:        input.HmebSeats,
	}
}

// MigrateRoomsRequestWith is a one-time backfill: it derives requestWith (and the
// matching needsRequest) for every room and persists it. Returns the number of
// rooms updated. Idempotent.
func (p *Plexams) MigrateRoomsRequestWith(ctx context.Context) (int, error) {
	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, room := range rooms {
		want := requestWithForRoom(room)
		if room.RequestWith == want {
			continue
		}
		if err := p.dbClient.SetRoomRequestWith(ctx, room.Name, string(want), want != model.RoomRequestTypeNone); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

// AddRoom creates a new room (key: name). Errors if a room with that name
// already exists.
func (p *Plexams) AddRoom(ctx context.Context, input model.RoomInput) (*model.Room, error) {
	exists, err := p.dbClient.HasRoom(ctx, input.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("room %s already exists", input.Name)
	}
	room, err := p.dbClient.AddRoom(ctx, roomInputToRoom(input))
	if err != nil {
		return nil, err
	}
	// keep the in-memory room-info map (built once at startup) in sync, so the
	// room is known to planning/validation without a server restart.
	p.roomInfo[room.Name] = room
	return room, nil
}

// UpdateRoom updates an existing room (key: name), keeping its active state.
// Errors if the room does not exist.
func (p *Plexams) UpdateRoom(ctx context.Context, input model.RoomInput) (*model.Room, error) {
	existing, err := p.dbClient.RoomByName(ctx, input.Name)
	if err != nil {
		return nil, err
	}
	updated := roomInputToRoom(input)
	updated.Deactivated = existing.Deactivated // toggle owns the active state
	room, err := p.dbClient.ReplaceRoom(ctx, updated)
	if err != nil {
		return nil, err
	}
	// keep the in-memory room-info map (built once at startup) in sync.
	p.roomInfo[room.Name] = room
	return room, nil
}

// RoomsForSlots computes the allowed rooms per slot live from the current state
// (global rooms, EXaHM/Anny bookings, building-management requests, room blocks).
// There is no stored cache anymore.
func (p *Plexams) RoomsForSlots(ctx context.Context) ([]*model.RoomsForSlot, error) {
	return p.computeRoomsForSlots(ctx, newDiscardReporter())
}

// RoomsForSlot returns the allowed rooms for a single slot (computed live). For
// loops, compute the map once via roomsForSlotsMap instead.
func (p *Plexams) RoomsForSlot(ctx context.Context, day int, time int) (*model.RoomsForSlot, error) {
	roomsForSlots, err := p.computeRoomsForSlots(ctx, newDiscardReporter())
	if err != nil {
		return nil, err
	}
	for _, rfs := range roomsForSlots {
		if rfs.Day == day && rfs.Slot == time {
			return rfs, nil
		}
	}
	return nil, nil
}

// roomsForSlotsMap computes the allowed rooms once and indexes them by slot, for
// callers that need many slots (validations, generation).
func (p *Plexams) roomsForSlotsMap(ctx context.Context) (map[SlotNumber][]string, error) {
	roomsForSlots, err := p.computeRoomsForSlots(ctx, newDiscardReporter())
	if err != nil {
		return nil, err
	}
	m := make(map[SlotNumber][]string, len(roomsForSlots))
	for _, rfs := range roomsForSlots {
		m[SlotNumber{day: rfs.Day, slot: rfs.Slot}] = rfs.RoomNames
	}
	return m, nil
}

func (p *Plexams) PreAddRoomToExam(ctx context.Context, ancode int, roomName string, mtknr *string, reserve bool, seats *int) (bool, error) {
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
		student, err := p.StudentByMtknr(ctx, *mtknr)
		if err != nil {
			log.Error().Err(err).Str("mtknr", *mtknr).Msg("cannot get student by mtknr")
			return false, err
		}
		if student == nil {
			log.Error().Str("mtknr", *mtknr).Msg("student not found")
			return false, fmt.Errorf("student with mtknr %s not found", *mtknr)
		}
		reserve = false // room for one student is NTA and never reserve
		seats = nil     // an NTA room is for exactly one student
	}

	if seats != nil {
		if *seats < 1 {
			return false, fmt.Errorf("seats must be at least 1")
		}
		if *seats > room.Seats {
			return false, fmt.Errorf("room %s has only %d seats, cannot plan %d", roomName, room.Seats, *seats)
		}
	}

	return p.dbClient.AddPrePlannedRoomToExam(ctx, &model.PrePlannedRoom{
		Ancode:   ancode,
		RoomName: roomName,
		Mtknr:    mtknr,
		Reserve:  reserve,
		Seats:    seats,
	})
}

func (p *Plexams) PrePlannedRooms(ctx context.Context) ([]*model.PrePlannedRoom, error) {
	return p.dbClient.PrePlannedRooms(ctx)
}

func (p *Plexams) PrePlannedRoomsForExam(ctx context.Context, ancode int) ([]*model.PrePlannedRoom, error) {
	return p.dbClient.PrePlannedRoomsForExam(ctx, ancode)
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

// UnplacedExams returns the students that could not be assigned a real room in
// their slot during the last room generation.
func (p *Plexams) UnplacedExams(ctx context.Context) ([]*model.UnplacedExam, error) {
	return p.dbClient.UnplacedExams(ctx)
}

func (p *Plexams) RoomByName(ctx context.Context, roomName string) (*model.Room, error) {
	room, err := p.dbClient.RoomByName(ctx, roomName)
	if err != nil {
		return nil, err
	}
	normalizeRoomRequestWith(room)
	return room, nil
}
