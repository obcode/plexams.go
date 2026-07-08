package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ExahmRoomsFromAnnyBookings(ctx context.Context) ([]anny.RoomBooking, error) {
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

	entries := make([]anny.RoomBooking, 0, len(dbBookings))
	for _, booking := range dbBookings {
		if booking.Room == "" {
			continue
		}
		if !anny.MatchesAnyPersonalization(booking.PersonalizationName, names) {
			continue
		}
		if !anny.IsApprovedStatus(booking.Status) {
			continue // a canceled/pending booking is not confirmed room capacity
		}
		normalizedRoom := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(booking.Room), " ", ""))
		if _, ok := allowedRooms[normalizedRoom]; !ok {
			continue
		}
		entries = append(entries, anny.RoomBooking{
			From:     booking.StartDate,
			Until:    booking.EndDate,
			Rooms:    []string{booking.Room},
			Approved: anny.IsApprovedStatus(booking.Status),
		})
	}

	return anny.MergeRoomBookings(entries), nil
}

type TimeRange struct {
	From      time.Time
	Until     time.Time
	Starttime *time.Time
	Approved  bool
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
			From:      r.From,
			Until:     r.Until,
			Starttime: r.Starttime,
			Approved:  r.Approved,
		})
	}
	return reservations, nil
}

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
func (p *Plexams) RoomsForSlot(ctx context.Context, starttime time.Time) (*model.RoomsForSlot, error) {
	roomsForSlots, err := p.computeRoomsForSlots(ctx, newDiscardReporter())
	if err != nil {
		return nil, err
	}
	for _, rfs := range roomsForSlots {
		if rfs.Starttime.Equal(starttime) {
			return rfs, nil
		}
	}
	return nil, nil
}

// roomsForSlotsMap computes the allowed rooms once and indexes them by the slot's
// absolute start time, for callers that need many slots (validations, generation).
func (p *Plexams) roomsForSlotsMap(ctx context.Context) (map[time.Time][]string, error) {
	roomsForSlots, err := p.computeRoomsForSlots(ctx, newDiscardReporter())
	if err != nil {
		return nil, err
	}
	m := make(map[time.Time][]string, len(roomsForSlots))
	for _, rfs := range roomsForSlots {
		m[rfs.Starttime] = rfs.RoomNames
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

func (p *Plexams) PlannedRoomNames(ctx context.Context) ([]string, error) {
	return p.dbClient.PlannedRoomNames(ctx)
}

func (p *Plexams) PlannedRoomsInSlot(ctx context.Context, starttime time.Time) ([]*model.PlannedRoom, error) {
	rooms, err := p.dbClient.PlannedRoomsAt(ctx, starttime)
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Msg("cannot get exams in slot")
	}

	return rooms, nil
}

// PlannedRoomsAt is an alias for PlannedRoomsInSlot keyed on the absolute start time.
func (p *Plexams) PlannedRoomsAt(ctx context.Context, starttime time.Time) ([]*model.PlannedRoom, error) {
	return p.PlannedRoomsInSlot(ctx, starttime)
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

	return nil, nil
}

func (p *Plexams) PlannedRoomNamesInSlot(ctx context.Context, starttime time.Time) ([]string, error) {
	return p.dbClient.PlannedRoomNamesAt(ctx, starttime)
}

// PlannedRoomNamesAt is an alias for PlannedRoomNamesInSlot keyed on the start time.
func (p *Plexams) PlannedRoomNamesAt(ctx context.Context, starttime time.Time) ([]string, error) {
	return p.dbClient.PlannedRoomNamesAt(ctx, starttime)
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
