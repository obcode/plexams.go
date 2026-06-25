package plexams

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// slotBooking is the EXaHM/SEB capacity already booked in Anny for one slot.
type slotBooking struct {
	exahmSeats int
	sebSeats   int
	rooms      map[string]bool // normalized names of rooms already booked for the slot
}

func normRoomName(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// annyBookedBySlot returns, for each given slot, the EXaHM/SEB seats already booked
// in Anny (a booking covers a slot when the slot's start time lies within the
// booking window). Used so the validation can show which bookings are still missing.
func (p *Plexams) annyBookedBySlot(ctx context.Context, slotKeys [][2]int) (map[[2]int]*slotBooking, error) {
	result := make(map[[2]int]*slotBooking, len(slotKeys))
	if len(slotKeys) == 0 {
		return result, nil
	}

	entries, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}

	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	roomByNorm := make(map[string]*model.Room, len(rooms))
	for _, r := range rooms {
		roomByNorm[normRoomName(r.Name)] = r
	}

	for _, key := range slotKeys {
		sb := &slotBooking{rooms: map[string]bool{}}
		result[key] = sb
		if len(entries) == 0 {
			continue
		}
		start, err := p.GetStarttime(key[0], key[1])
		if err != nil || start == nil {
			continue
		}
		for _, e := range entries {
			// slot start within [From, Until)
			if start.Before(e.From) || !start.Before(e.Until) {
				continue
			}
			for _, rn := range e.Rooms {
				n := normRoomName(rn)
				if sb.rooms[n] {
					continue
				}
				room := roomByNorm[n]
				if room == nil || room.Deactivated {
					continue
				}
				if room.Exahm {
					sb.exahmSeats += room.Seats
				}
				if room.Seb {
					seats := room.Seats
					if room.SebSeats != nil {
						seats = *room.SebSeats
					}
					sb.sebSeats += seats
				}
				sb.rooms[n] = true
			}
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
