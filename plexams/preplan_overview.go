package plexams

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// roomCapacity is one room usable for a given kind, with its seat count.
type roomCapacity struct {
	name  string
	seats int
}

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

	// group pre-exams by slot key; "" = unslotted
	type slotKey struct {
		day, slot int
		slotted   bool
	}
	groups := make(map[slotKey][]*model.PreplanExam)
	keys := make([]slotKey, 0)
	for _, pe := range preExams {
		var key slotKey
		if pe.PlannedDayNumber != nil && pe.PlannedSlotNumber != nil {
			key = slotKey{day: *pe.PlannedDayNumber, slot: *pe.PlannedSlotNumber, slotted: true}
		}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], pe)
	}

	// stable order: slotted by day/slot ascending, unslotted bucket last
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].slotted != keys[j].slotted {
			return keys[i].slotted // slotted first
		}
		if keys[i].day != keys[j].day {
			return keys[i].day < keys[j].day
		}
		return keys[i].slot < keys[j].slot
	})

	// Anny bookings already made for the slotted slots, so we can show what is
	// still missing.
	slotKeys := make([][2]int, 0)
	for _, key := range keys {
		if key.slotted {
			slotKeys = append(slotKeys, [2]int{key.day, key.slot})
		}
	}
	booked, err := p.annyBookedBySlot(ctx, slotKeys)
	if err != nil {
		return nil, err
	}

	slots := make([]*model.PreplanSlotNeed, 0, len(keys))
	for _, key := range keys {
		exams := groups[key]

		need := &model.PreplanSlotNeed{
			Exahm:     kindNeed(exams, "EXaHM", exahmRooms),
			Seb:       kindNeed(exams, "SEB", sebRooms),
			Conflicts: programConflicts(exams),
		}
		if key.slotted {
			day, slot := key.day, key.slot
			need.DayNumber = &day
			need.SlotNumber = &slot
			if start, err := p.GetStarttime(day, slot); err == nil {
				need.Starttime = start
			}
			if sb := booked[[2]int{day, slot}]; sb != nil {
				applyBooking(need.Exahm, sb.exahmSeats, roomsForKind(exams, "EXaHM", exahmRooms), sb.rooms)
				applyBooking(need.Seb, sb.sebSeats, roomsForKind(exams, "SEB", sebRooms), sb.rooms)
			}
		}
		slots = append(slots, need)
	}

	return &model.PreplanOverview{Slots: slots}, nil
}

// applyBooking fills the booked seats and the still-to-book rooms for one kind.
func applyBooking(need *model.PreplanKindNeed, bookedSeats int, rooms []roomCapacity, bookedRooms map[string]bool) {
	need.SeatsBooked = bookedSeats
	gap := need.SeatsNeeded - bookedSeats
	if gap < 0 {
		gap = 0
	}
	need.RoomsToBook = roomsToBook(rooms, gap, bookedRooms)
}

// preplanRoomCapacities returns the usable EXaHM and SEB rooms (sorted by seats
// descending). EXaHM rooms use Seats; SEB rooms use SebSeats when set, else Seats.
func (p *Plexams) preplanRoomCapacities(ctx context.Context) (exahm, seb []roomCapacity, err error) {
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
			exahm = append(exahm, roomCapacity{name: room.Name, seats: room.Seats})
		}
		if room.Seb {
			seats := room.Seats
			if room.SebSeats != nil {
				seats = *room.SebSeats
			}
			seb = append(seb, roomCapacity{name: room.Name, seats: seats})
		}
	}
	sort.Slice(exahm, func(i, j int) bool { return exahm[i].seats > exahm[j].seats })
	sort.Slice(seb, func(i, j int) bool { return seb[i].seats > seb[j].seats })
	return exahm, seb, nil
}

func totalSeats(rooms []roomCapacity) int {
	total := 0
	for _, r := range rooms {
		total += r.seats
	}
	return total
}

// kindNeed sums the seat demand of the pre-exams of one kind in a slot and greedily
// picks rooms (largest first) to cover it, honouring per-exam room restrictions.
func kindNeed(exams []*model.PreplanExam, kind string, rooms []roomCapacity) *model.PreplanKindNeed {
	count, seats := 0, 0
	for _, pe := range exams {
		if pe.ExamKind == kind {
			count++
			seats += pe.ExpectedStudents
		}
	}

	pool := roomsForKind(exams, kind, rooms)
	available := totalSeats(pool)

	roomNames := make([]string, 0)
	remaining := seats
	for _, r := range pool {
		if remaining <= 0 {
			break
		}
		roomNames = append(roomNames, r.name)
		remaining -= r.seats
	}

	return &model.PreplanKindNeed{
		ExamCount:      count,
		SeatsNeeded:    seats,
		RoomsSuggested: len(roomNames),
		Rooms:          roomNames,
		SeatsAvailable: available,
		SeatsBooked:    0,
		RoomsToBook:    []string{},
	}
}

// roomsForKind restricts the candidate rooms by the per-exam allowedRooms of the
// slot's exams of that kind: only when every such exam restricts its rooms is the
// pool narrowed to the union of their allowedRooms (an exam without a restriction
// may use any room, so the full set is kept).
func roomsForKind(exams []*model.PreplanExam, kind string, rooms []roomCapacity) []roomCapacity {
	allowed := make(map[string]bool)
	hasRestriction, hasUnrestricted := false, false
	for _, pe := range exams {
		if pe.ExamKind != kind {
			continue
		}
		var ar []string
		if pe.Constraints != nil && pe.Constraints.RoomConstraints != nil {
			ar = pe.Constraints.RoomConstraints.AllowedRooms
		}
		if len(ar) == 0 {
			hasUnrestricted = true
			continue
		}
		hasRestriction = true
		for _, r := range ar {
			allowed[normRoomName(r)] = true
		}
	}
	if !hasRestriction || hasUnrestricted {
		return rooms
	}
	filtered := make([]roomCapacity, 0, len(rooms))
	for _, r := range rooms {
		if allowed[normRoomName(r.name)] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// programConflicts finds study programs that appear in more than one pre-exam of
// the same slot (a possible student clash, since Primuss conflicts aren't known yet).
func programConflicts(exams []*model.PreplanExam) []*model.PreplanProgramConflict {
	type acc struct {
		ids     []int
		modules []string
	}
	byProgram := make(map[string]*acc)
	order := make([]string, 0)
	for _, pe := range exams {
		for _, prog := range pe.Programs {
			a, ok := byProgram[prog]
			if !ok {
				a = &acc{}
				byProgram[prog] = a
				order = append(order, prog)
			}
			a.ids = append(a.ids, pe.ID)
			a.modules = append(a.modules, pe.Module)
		}
	}

	conflicts := make([]*model.PreplanProgramConflict, 0)
	sort.Strings(order)
	for _, prog := range order {
		a := byProgram[prog]
		if len(a.ids) > 1 {
			conflicts = append(conflicts, &model.PreplanProgramConflict{
				Program:        prog,
				PreplanExamIDs: a.ids,
				Modules:        a.modules,
			})
		}
	}
	return conflicts
}
