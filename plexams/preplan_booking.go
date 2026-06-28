package plexams

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// slotBooking is the EXaHM capacity WE have already booked in Anny for one slot.
// (SEB runs in labs, not booked via Anny, so it is not tracked here.)
type slotBooking struct {
	exahmSeats int
	rooms      map[string]bool // normalized names of rooms already booked for the slot
}

func normRoomName(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// annyBookedBySlot returns, for each given slot, the EXaHM seats we have already
// booked in Anny in the T-building rooms (a booking covers a slot when the slot's
// start time lies within the booking window). Only our own bookings count (matched
// by the configured personalization names; all bookings when none are configured).
func (p *Plexams) annyBookedBySlot(ctx context.Context, slotKeys [][2]int) (map[[2]int]*slotBooking, error) {
	result := make(map[[2]int]*slotBooking, len(slotKeys))
	if len(slotKeys) == 0 {
		return result, nil
	}

	bookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}
	names := p.annyPersonalizationNames(ctx)

	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	// only EXaHM-capable T-building (Anny) rooms count toward the booked capacity
	exahmAnny := make(map[string]*model.Room, len(rooms))
	for _, r := range rooms {
		if r.Deactivated {
			continue
		}
		if r.Exahm && r.RequestWith == model.RoomRequestTypeAnny {
			exahmAnny[normRoomName(r.Name)] = r
		}
	}

	for _, key := range slotKeys {
		sb := &slotBooking{rooms: map[string]bool{}}
		result[key] = sb
		start, err := p.GetStarttime(key[0], key[1])
		if err != nil || start == nil {
			continue
		}
		for _, b := range bookings {
			if b.Room == "" {
				continue
			}
			if !matchesAnyPersonalization(b.PersonalizationName, names) {
				continue // only our bookings
			}
			// slot start within [start, end) of the booking
			if start.Before(b.StartDate) || !start.Before(b.EndDate) {
				continue
			}
			n := normRoomName(b.Room)
			if sb.rooms[n] {
				continue
			}
			room := exahmAnny[n]
			if room == nil {
				continue
			}
			sb.exahmSeats += room.Seats
			sb.rooms[n] = true
		}
	}
	return result, nil
}

// roomsToBook greedily picks rooms (largest first) that are NOT yet booked for the
// slot, enough to cover the still-missing seats (gap). Empty when nothing is missing.
func roomsToBook(rooms []roomCapacity, gap int, booked map[string]bool) []string {
	names := make([]string, 0)
	remaining := gap
	for _, r := range rooms {
		if remaining <= 0 {
			break
		}
		if booked != nil && booked[normRoomName(r.name)] {
			continue
		}
		names = append(names, r.name)
		remaining -= r.seats
	}
	return names
}
