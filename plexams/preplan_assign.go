package plexams

import (
	"context"
	"fmt"
	"math/rand"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// cost weights for the pre-exam assignment: a shared study program in a slot
// (the only conflict signal we have without Primuss data) dominates; seat
// overflow beyond the EXaHM/SEB room capacity is a softer secondary penalty.
const (
	preplanConflictWeight = 1000
	preplanOverflowWeight = 1
)

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
	exahmAvail, sebAvail := totalSeats(exahmRooms), totalSeats(sebRooms)
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
		messages = append(messages, kindBookingMessages(key, "EXaHM", exahm, exahmAvail, exahmRooms, sb)...)
		messages = append(messages, kindBookingMessages(key, "SEB", seb, sebAvail, sebRooms, sb)...)

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

// GeneratePreplanAssignment assigns the pre-exams to the EXaHM/SEB go-slots,
// minimizing shared study programs per slot and seat overflow (greedy, largest
// first, then a short hill-climbing polish). With keepAssigned the already-slotted
// exams stay put and only the unassigned ones are placed. The new assignment is
// persisted; the result is its validation.
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
	exahmAvail, sebAvail := totalSeats(exahmRooms), totalSeats(sebRooms)

	// candidate slots = the MUC.DAI slots
	type slotRef struct{ day, slot int }
	slots := make([]slotRef, 0, len(mucDaiSlots))
	for _, s := range mucDaiSlots {
		slots = append(slots, slotRef{day: s.DayNumber, slot: s.SlotNumber})
	}

	// assignment: index into slots per exam, or -1 = unassigned/fixed-elsewhere.
	// fixed[i] true => exam i keeps its current slot and is not moved.
	assign := make([]int, len(preExams))
	fixed := make([]bool, len(preExams))
	slotByKey := make(map[slotRef]int, len(slots))
	for i, s := range slots {
		slotByKey[s] = i
	}
	for i, pe := range preExams {
		assign[i] = -1
		if keepAssigned && pe.PlannedDayNumber != nil && pe.PlannedSlotNumber != nil {
			fixed[i] = true
			if idx, ok := slotByKey[slotRef{*pe.PlannedDayNumber, *pe.PlannedSlotNumber}]; ok {
				assign[i] = idx
			} else {
				// fixed to a non-go-slot: leave it where it is, exclude from cost slots
				fixed[i] = true
			}
		}
	}

	// cost over the candidate slots given the current assignment
	cost := func(a []int) int {
		type load struct {
			exahm, seb int
			progs      map[string]int
		}
		perSlot := make(map[int]*load)
		for i, pe := range preExams {
			si := a[i]
			if si < 0 {
				continue
			}
			l := perSlot[si]
			if l == nil {
				l = &load{progs: map[string]int{}}
				perSlot[si] = l
			}
			switch pe.ExamKind {
			case "EXaHM":
				l.exahm += pe.ExpectedStudents
			case "SEB":
				l.seb += pe.ExpectedStudents
			}
			for _, prog := range pe.Programs {
				l.progs[prog]++
			}
		}
		total := 0
		for _, l := range perSlot {
			for _, cnt := range l.progs {
				if cnt > 1 {
					total += preplanConflictWeight * (cnt - 1)
				}
			}
			if l.exahm > exahmAvail {
				total += preplanOverflowWeight * (l.exahm - exahmAvail)
			}
			if l.seb > sebAvail {
				total += preplanOverflowWeight * (l.seb - sebAvail)
			}
		}
		return total
	}

	// movable exams, largest first (better packing)
	movable := make([]int, 0, len(preExams))
	for i := range preExams {
		if !fixed[i] {
			movable = append(movable, i)
		}
	}
	sort.Slice(movable, func(a, b int) bool {
		if preExams[movable[a]].ExpectedStudents != preExams[movable[b]].ExpectedStudents {
			return preExams[movable[a]].ExpectedStudents > preExams[movable[b]].ExpectedStudents
		}
		return preExams[movable[a]].ID < preExams[movable[b]].ID
	})

	// greedy: place each movable exam in the slot with the smallest marginal cost
	for _, i := range movable {
		bestSlot, bestCost := -1, 0
		for si := range slots {
			assign[i] = si
			c := cost(assign)
			if bestSlot == -1 || c < bestCost {
				bestSlot, bestCost = si, c
			}
		}
		assign[i] = bestSlot
	}

	// short hill-climbing polish (deterministic seed)
	if len(movable) > 0 {
		rng := rand.New(rand.NewSource(1)) //nolint:gosec // not security relevant
		current := cost(assign)
		iterations := 4000
		for it := 0; it < iterations; it++ {
			i := movable[rng.Intn(len(movable))]
			old := assign[i]
			cand := rng.Intn(len(slots))
			if cand == old {
				continue
			}
			assign[i] = cand
			c := cost(assign)
			if c <= current {
				current = c
			} else {
				assign[i] = old
			}
		}
	}

	// persist
	for i, pe := range preExams {
		if fixed[i] {
			continue
		}
		if assign[i] < 0 {
			pe.PlannedDayNumber = nil
			pe.PlannedSlotNumber = nil
		} else {
			day, slot := slots[assign[i]].day, slots[assign[i]].slot
			pe.PlannedDayNumber = &day
			pe.PlannedSlotNumber = &slot
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
	booked, err := p.annyBookedBySlot(ctx, slotKeys)
	if err != nil {
		return nil, err
	}

	return validatePreplan(preExams, exahmRooms, sebRooms, booked), nil
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
