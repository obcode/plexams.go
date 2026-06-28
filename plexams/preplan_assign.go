package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// preplanCapacityFactor leaves ~10% of a slot's booked Anny seats free instead of
// filling rooms to the brim (e.g. ~108 of ~120 seats in the 4 T-building rooms).
const preplanCapacityFactor = 0.9

// ValidatePreplanAssignment checks the current slot assignment of the pre-exams:
// unassigned exams, per-slot/kind capacity overflow, and study programs shared by
// more than one exam in the same slot. ok is true when there are no findings.
func (p *Plexams) ValidatePreplanAssignment(ctx context.Context) (*model.PreplanValidation, error) {
	preExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	exahmRooms, sebRooms, err := p.preplanRoomCapacities(ctx)
	if err != nil {
		return nil, err
	}

	slotKeys := make([][2]int, 0)
	for _, pe := range preExams {
		if pe.PlannedDayNumber != nil && pe.PlannedSlotNumber != nil {
			slotKeys = append(slotKeys, [2]int{*pe.PlannedDayNumber, *pe.PlannedSlotNumber})
		}
	}
	booked, err := p.annyBookedBySlot(ctx, slotKeys)
	if err != nil {
		return nil, err
	}

	return validatePreplan(preExams, exahmRooms, sebRooms, booked), nil
}

// validatePreplan builds the validation result from an in-memory set of pre-exams.
// booked (Anny bookings per slot) may be nil; when present, missing bookings are
// reported so the planner can book step by step.
func validatePreplan(preExams []*model.PreplanExam, exahmRooms, sebRooms []roomCapacity, booked map[[2]int]*slotBooking) *model.PreplanValidation {
	messages := make([]string, 0)

	unassigned := make([]int, 0)
	bySlot := make(map[[2]int][]*model.PreplanExam)
	slotOrder := make([][2]int, 0)
	for _, pe := range preExams {
		if pe.PlannedDayNumber == nil || pe.PlannedSlotNumber == nil {
			unassigned = append(unassigned, pe.ID)
			continue
		}
		key := [2]int{*pe.PlannedDayNumber, *pe.PlannedSlotNumber}
		if _, ok := bySlot[key]; !ok {
			slotOrder = append(slotOrder, key)
		}
		bySlot[key] = append(bySlot[key], pe)
	}
	sort.Slice(slotOrder, func(i, j int) bool {
		if slotOrder[i][0] != slotOrder[j][0] {
			return slotOrder[i][0] < slotOrder[j][0]
		}
		return slotOrder[i][1] < slotOrder[j][1]
	})

	if len(unassigned) > 0 {
		messages = append(messages, fmt.Sprintf("%s ohne Slot", pluralN(len(unassigned), "Prüfung", "Prüfungen")))
	}

	for _, key := range slotOrder {
		exams := bySlot[key]
		exahm, seb := 0, 0
		for _, pe := range exams {
			switch pe.ExamKind {
			case "EXaHM":
				exahm += pe.ExpectedStudents
			case "SEB":
				seb += pe.ExpectedStudents
			}
		}

		var sb *slotBooking
		if booked != nil {
			sb = booked[key]
		}
		// honour per-exam room restrictions for the suggestion/capacity
		exahmPool := roomsForKind(exams, "EXaHM", exahmRooms)
		sebPool := roomsForKind(exams, "SEB", sebRooms)
		messages = append(messages, kindBookingMessages(key, "EXaHM", exahm, totalSeats(exahmPool), exahmPool, sb)...)
		messages = append(messages, kindBookingMessages(key, "SEB", seb, totalSeats(sebPool), sebPool, sb)...)

		for _, c := range programConflicts(exams) {
			messages = append(messages, fmt.Sprintf("Slot %d/%d: Studiengang %s in %d Prüfungen (%s)",
				key[0], key[1], c.Program, len(c.PreplanExamIDs), joinStrings(c.Modules)))
		}
	}

	assignedCount := len(preExams) - len(unassigned)
	return &model.PreplanValidation{
		Ok:            len(messages) == 0,
		AssignedCount: assignedCount,
		UnassignedIDs: unassigned,
		Messages:      messages,
	}
}

// kindBookingMessages reports, for one slot and kind, a physical-capacity overflow
// (can't fit even fully booked) or — when within capacity — the Anny bookings still
// missing to cover the demand.
func kindBookingMessages(key [2]int, kind string, needed, available int, rooms []roomCapacity, sb *slotBooking) []string {
	if needed == 0 {
		return nil
	}
	if needed > available {
		return []string{fmt.Sprintf("Slot %d/%d: %s %d Plätze nötig, nur %d verfügbar (Kapazität reicht nicht)",
			key[0], key[1], kind, needed, available)}
	}

	bookedSeats := 0
	var bookedRooms map[string]bool
	if sb != nil {
		bookedRooms = sb.rooms
		if kind == "EXaHM" {
			bookedSeats = sb.exahmSeats
		} else {
			bookedSeats = sb.sebSeats
		}
	}
	if bookedSeats >= needed {
		return nil
	}

	toBook := roomsToBook(rooms, needed-bookedSeats, bookedRooms)
	msg := fmt.Sprintf("Slot %d/%d: %s noch %d Plätze zu buchen (gebucht %d von %d nötig)",
		key[0], key[1], kind, needed-bookedSeats, bookedSeats, needed)
	if len(toBook) > 0 {
		msg += " — z. B. " + joinStrings(toBook)
	}
	return []string{msg}
}

// GeneratePreplanAssignment distributes the pre-exams over the MUC.DAI slots that
// already have Anny rooms booked, up to ~90% of each slot's booked capacity (never
// brim-full). The most important exams are placed first (EXaHM, then large SEB);
// small SEB that no longer fit are left without a slot. Exams of the same study
// program never share a slot and are spread across different days where possible.
// Same-slot exams stay together. When no Anny rooms are booked anywhere, nothing is
// assigned. With keepAssigned, exams already sitting in a booked slot keep it.
func (p *Plexams) GeneratePreplanAssignment(ctx context.Context, keepAssigned bool) (*model.PreplanValidation, error) {
	preExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	if len(preExams) == 0 {
		return &model.PreplanValidation{Ok: true, Messages: []string{}, UnassignedIDs: []int{}}, nil
	}
	mucDaiSlots := p.semesterConfig.MucDaiSlots
	if len(mucDaiSlots) == 0 {
		return nil, fmt.Errorf("no MUC.DAI slots configured for this semester")
	}
	exahmRooms, sebRooms, err := p.preplanRoomCapacities(ctx)
	if err != nil {
		return nil, err
	}

	// booked Anny capacity per MUC.DAI slot
	allKeys := make([][2]int, 0, len(mucDaiSlots))
	for _, s := range mucDaiSlots {
		allKeys = append(allKeys, [2]int{s.DayNumber, s.SlotNumber})
	}
	booked, err := p.annyBookedBySlot(ctx, allKeys)
	if err != nil {
		return nil, err
	}

	// candidate slots: only those with booked Anny rooms; usable capacity = 90%
	type cslot struct {
		day, slotNo int
		capacity    int
		used        int
		programs    map[string]bool
	}
	slots := make([]*cslot, 0)
	slotByKey := make(map[[2]int]*cslot)
	for _, s := range mucDaiSlots {
		sb := booked[[2]int{s.DayNumber, s.SlotNumber}]
		if sb == nil {
			continue
		}
		capacity := int(float64(sb.seats) * preplanCapacityFactor)
		if capacity <= 0 {
			continue
		}
		cs := &cslot{day: s.DayNumber, slotNo: s.SlotNumber, capacity: capacity, programs: map[string]bool{}}
		slots = append(slots, cs)
		slotByKey[[2]int{s.DayNumber, s.SlotNumber}] = cs
	}

	// the booked slot all members of a unit currently sit in, or nil otherwise
	currentUnitSlot := func(members []int) *cslot {
		var slot *cslot
		for _, i := range members {
			pe := preExams[i]
			if pe.PlannedDayNumber == nil || pe.PlannedSlotNumber == nil {
				return nil
			}
			cs := slotByKey[[2]int{*pe.PlannedDayNumber, *pe.PlannedSlotNumber}]
			if cs == nil {
				return nil
			}
			if slot == nil {
				slot = cs
			} else if slot != cs {
				return nil
			}
		}
		return slot
	}

	programDays := make(map[string]map[int]bool) // program -> days it already occupies

	occupy := func(s *cslot, seats int, programs map[string]bool) {
		s.used += seats
		for prog := range programs {
			s.programs[prog] = true
			if programDays[prog] == nil {
				programDays[prog] = map[int]bool{}
			}
			programDays[prog][s.day] = true
		}
	}

	// same-slot groups (union-find over indices)
	idToIdx := make(map[int]int, len(preExams))
	for i, pe := range preExams {
		idToIdx[pe.ID] = i
	}
	parent := make([]int, len(preExams))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i, pe := range preExams {
		if pe.Constraints == nil {
			continue
		}
		for _, otherID := range pe.Constraints.SameSlot {
			if j, ok := idToIdx[otherID]; ok {
				parent[find(i)] = find(j)
			}
		}
	}

	type unit struct {
		members  []int
		seats    int
		programs map[string]bool
		hasExahm bool
		minID    int
	}
	groups := make(map[int]*unit)
	for i, pe := range preExams {
		r := find(i)
		u := groups[r]
		if u == nil {
			u = &unit{programs: map[string]bool{}, minID: pe.ID}
			groups[r] = u
		}
		u.members = append(u.members, i)
		u.seats += pe.ExpectedStudents
		if pe.ExamKind == "EXaHM" {
			u.hasExahm = true
		}
		for _, prog := range pe.Programs {
			u.programs[prog] = true
		}
		if pe.ID < u.minID {
			u.minID = pe.ID
		}
	}

	// keepAssigned: units already sitting in a booked slot keep it (pre-occupy)
	assignSlot := make(map[int]*cslot)
	units := make([]*unit, 0, len(groups))
	for _, u := range groups {
		if keepAssigned {
			if cs := currentUnitSlot(u.members); cs != nil {
				occupy(cs, u.seats, u.programs)
				for _, i := range u.members {
					assignSlot[i] = cs
				}
				continue
			}
		}
		units = append(units, u)
	}

	// priority: EXaHM units first, then largest first (so small SEB are placed last)
	sort.Slice(units, func(a, b int) bool {
		if units[a].hasExahm != units[b].hasExahm {
			return units[a].hasExahm
		}
		if units[a].seats != units[b].seats {
			return units[a].seats > units[b].seats
		}
		return units[a].minID < units[b].minID
	})

	for _, u := range units {
		var best *cslot
		bestPenalty := 0
		for _, s := range slots {
			if s.capacity-s.used < u.seats {
				continue // would exceed the 90% booked capacity
			}
			if overlapsProgram(u.programs, s.programs) {
				continue // same study program never in the same slot
			}
			penalty := dayClashes(u.programs, programDays, s.day) // prefer different days
			free := s.capacity - s.used
			if best == nil || penalty < bestPenalty ||
				(penalty == bestPenalty && free > best.capacity-best.used) {
				best, bestPenalty = s, penalty
			}
		}
		if best == nil {
			continue // no booked slot with room for it → leave unassigned
		}
		occupy(best, u.seats, u.programs)
		for _, i := range u.members {
			assignSlot[i] = best
		}
	}

	// persist
	for i, pe := range preExams {
		if s := assignSlot[i]; s != nil {
			day, slot := s.day, s.slotNo
			pe.PlannedDayNumber = &day
			pe.PlannedSlotNumber = &slot
		} else {
			pe.PlannedDayNumber = nil
			pe.PlannedSlotNumber = nil
		}
		if _, err := p.dbClient.ReplacePreplanExam(ctx, pe); err != nil {
			return nil, err
		}
	}

	slotKeys := make([][2]int, 0)
	for _, pe := range preExams {
		if pe.PlannedDayNumber != nil && pe.PlannedSlotNumber != nil {
			slotKeys = append(slotKeys, [2]int{*pe.PlannedDayNumber, *pe.PlannedSlotNumber})
		}
	}
	bookedAfter, err := p.annyBookedBySlot(ctx, slotKeys)
	if err != nil {
		return nil, err
	}
	result := validatePreplan(preExams, exahmRooms, sebRooms, bookedAfter)
	if len(slots) == 0 {
		result.Ok = false
		result.Messages = append([]string{"keine Anny-Räume gebucht — nichts zugeordnet (zuerst Anny-Räume buchen und importieren)"}, result.Messages...)
	}
	return result, nil
}

func overlapsProgram(a, b map[string]bool) bool {
	for prog := range a {
		if b[prog] {
			return true
		}
	}
	return false
}

// dayClashes counts how many of the unit's programs already occupy the given day.
func dayClashes(programs map[string]bool, programDays map[string]map[int]bool, day int) int {
	n := 0
	for prog := range programs {
		if programDays[prog] != nil && programDays[prog][day] {
			n++
		}
	}
	return n
}

func joinStrings(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
