package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/obcode/plexams.go/plexams/preplancalc"
)

// slotBooking is the EXaHM/SEB capacity WE have already booked in Anny (T-building)
// for one slot.
type slotBooking struct {
	exahmSeats int
	sebSeats   int
	seats      int             // total physical seats of the booked rooms (each room once)
	rooms      map[string]bool // normalized names of rooms already booked for the slot
}

// annyBookedBySlot returns, for each given slot, the EXaHM/SEB seats we have already
// booked in Anny in the T-building rooms (a booking covers a slot when the slot's
// start time lies within the booking window). Only our own bookings count (matched
// by the configured personalization names; all bookings when none are configured).
// slotBlockDuration is the length of a slot block (spacing between consecutive start
// times, e.g. 2h), used to check a booking covers a whole slot. Defaults to 120 min.
func slotBlockDuration(starttimes []*model.Starttime) time.Duration {
	block := 120 * time.Minute
	if len(starttimes) >= 2 {
		t0, e0 := time.Parse("15:04", starttimes[0].Start)
		t1, e1 := time.Parse("15:04", starttimes[1].Start)
		if e0 == nil && e1 == nil && t1.After(t0) {
			block = t1.Sub(t0)
		}
	}
	return block
}

func (p *Plexams) annyBookedBySlot(ctx context.Context, slotKeys [][2]int) (map[[2]int]*slotBooking, error) {
	result := make(map[[2]int]*slotBooking, len(slotKeys))
	if len(slotKeys) == 0 {
		return result, nil
	}

	bookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}
	names := p.anny.PersonalizationNames(ctx)

	// a booking must cover the whole slot block (e.g. a 12:30 exam in a 2h slot runs until
	// 14:30), not just the slot start — otherwise a room booked only until 13:30 would be
	// wrongly counted as available for the 12:30 slot.
	block := slotBlockDuration(p.semesterConfig.Starttimes)

	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	// only T-building (Anny) rooms count toward the booked capacity
	annyRoom := make(map[string]*model.Room, len(rooms))
	for _, r := range rooms {
		if !r.Deactivated && r.RequestWith == model.RoomRequestTypeAnny {
			annyRoom[preplancalc.NormRoomName(r.Name)] = r
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
			if !anny.MatchesAnyPersonalization(b.PersonalizationName, names) {
				continue // only our bookings
			}
			// the booking must fully cover the slot block [start, start+block]
			if start.Before(b.StartDate) || start.Add(block).After(b.EndDate) {
				continue
			}
			n := preplancalc.NormRoomName(b.Room)
			if sb.rooms[n] {
				continue
			}
			room := annyRoom[n]
			if room == nil {
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
			sb.seats += room.Seats // physical seats, each booked room once
			sb.rooms[n] = true
		}
	}
	return result, nil
}
