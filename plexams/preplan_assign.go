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
	return validatePreplan(preExams, totalSeats(exahmRooms), totalSeats(sebRooms)), nil
}

// validatePreplan builds the validation result from an in-memory set of pre-exams.
func validatePreplan(preExams []*model.PreplanExam, exahmAvail, sebAvail int) *model.PreplanValidation {
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
		if exahm > exahmAvail {
			messages = append(messages, fmt.Sprintf("Slot %d/%d: EXaHM %d Plätze nötig, nur %d verfügbar",
				key[0], key[1], exahm, exahmAvail))
		}
		if seb > sebAvail {
			messages = append(messages, fmt.Sprintf("Slot %d/%d: SEB %d Plätze nötig, nur %d verfügbar",
				key[0], key[1], seb, sebAvail))
		}
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

	goSlots := p.semesterConfig.GoSlots
	if len(goSlots) == 0 {
		return nil, fmt.Errorf("no go-slots configured for this semester")
	}
	exahmRooms, sebRooms, err := p.preplanRoomCapacities(ctx)
	if err != nil {
		return nil, err
	}
	exahmAvail, sebAvail := totalSeats(exahmRooms), totalSeats(sebRooms)

	// candidate slots = the go-slots
	type slotRef struct{ day, slot int }
	slots := make([]slotRef, 0, len(goSlots))
	for _, s := range goSlots {
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

	return validatePreplan(preExams, exahmAvail, sebAvail), nil
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
