package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/preplancalc"
)

// PreplanOverview computes, per slot (plus one bucket for unslotted pre-exams),
// the EXaHM/SEB seat demand with a suggested set of rooms, and the program
// overlaps within each slot.
func (p *Plexams) PreplanOverview(ctx context.Context) (*model.PreplanOverview, error) {
	preExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}

	exahmRooms, sebRooms, err := p.preplanRoomCapacities(ctx)
	if err != nil {
		return nil, err
	}

	// group pre-exams by start time; zero time = unslotted
	type slotKey struct {
		start   time.Time
		slotted bool
	}
	groups := make(map[slotKey][]*model.PreplanExam)
	keys := make([]slotKey, 0)
	for _, pe := range preExams {
		var key slotKey
		if pe.PlannedStarttime != nil {
			key = slotKey{start: *pe.PlannedStarttime, slotted: true}
		}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], pe)
	}

	// stable order: slotted by start time ascending, unslotted bucket last
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].slotted != keys[j].slotted {
			return keys[i].slotted // slotted first
		}
		return keys[i].start.Before(keys[j].start)
	})

	// Anny bookings already made for the slotted slots, so we can show what is
	// still missing.
	starts := make([]time.Time, 0)
	for _, key := range keys {
		if key.slotted {
			starts = append(starts, key.start)
		}
	}
	booked, err := p.annyBookedByTime(ctx, starts)
	if err != nil {
		return nil, err
	}

	slots := make([]*model.PreplanSlotNeed, 0, len(keys))
	for _, key := range keys {
		exams := groups[key]

		need := &model.PreplanSlotNeed{
			Exahm:     preplancalc.KindNeed(exams, "EXaHM", exahmRooms),
			Seb:       preplancalc.KindNeed(exams, "SEB", sebRooms),
			Conflicts: preplancalc.ProgramConflicts(exams),
		}
		if key.slotted {
			start := key.start
			need.Starttime = &start
			if sb := booked[key.start]; sb != nil {
				preplancalc.ApplyBooking(need.Exahm, sb.exahmSeats, preplancalc.RoomsForKind(exams, "EXaHM", exahmRooms), sb.rooms)
				preplancalc.ApplyBooking(need.Seb, sb.sebSeats, preplancalc.RoomsForKind(exams, "SEB", sebRooms), sb.rooms)
			}
		}
		slots = append(slots, need)
	}

	return &model.PreplanOverview{Slots: slots}, nil
}

// preplanRoomCapacities returns the usable EXaHM and SEB rooms (sorted by seats
// descending). EXaHM rooms use Seats; SEB rooms use SebSeats when set, else Seats.
func (p *Plexams) preplanRoomCapacities(ctx context.Context) (exahm, seb []preplancalc.RoomCapacity, err error) {
	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, room := range rooms {
		if room.Deactivated {
			continue
		}
		// pre-planning only distributes into the T-building (Anny) rooms — for both
		// EXaHM and SEB. (Labs may be used later during real room planning, but Anny
		// rooms always have priority.)
		if room.RequestWith != model.RoomRequestTypeAnny {
			continue
		}
		if room.Exahm {
			exahm = append(exahm, preplancalc.RoomCapacity{Name: room.Name, Seats: room.Seats})
		}
		if room.Seb {
			seats := room.Seats
			if room.SebSeats != nil {
				seats = *room.SebSeats
			}
			seb = append(seb, preplancalc.RoomCapacity{Name: room.Name, Seats: seats})
		}
	}
	sort.Slice(exahm, func(i, j int) bool { return exahm[i].Seats > exahm[j].Seats })
	sort.Slice(seb, func(i, j int) bool { return seb[i].Seats > seb[j].Seats })
	return exahm, seb, nil
}

// nonAnnySebSeats returns the TOTAL SEB seats available in R-building labs (non-Anny SEB
// rooms). A SEB exam can be split across several labs, so a SEB pre-exam whose size fits
// this total can be planned in the R-building instead of consuming a booked Anny slot —
// leaving it out of Anny is then only a warning (see validatePreplan).
func (p *Plexams) nonAnnySebSeats(ctx context.Context) (int, error) {
	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, room := range rooms {
		if room.Deactivated || !room.Seb || room.RequestWith == model.RoomRequestTypeAnny {
			continue
		}
		seats := room.Seats
		if room.SebSeats != nil {
			seats = *room.SebSeats
		}
		total += seats
	}
	return total, nil
}
