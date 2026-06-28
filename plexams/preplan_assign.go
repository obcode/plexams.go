package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// preplanCapacityFactor is the usable fraction of a slot's booked Anny seats. 1.0 =
// fill the booked rooms completely (no reserve).
const preplanCapacityFactor = 1.0

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
// brim-full). Exams of the same study program never share a slot and are spread
// across days; the most important exams (EXaHM, then large SEB) are placed first, so
// small SEB that no longer fit are the ones left without a slot. Same-slot exams stay
// together. A DSATUR constructive pass plus an SA repair (see solvePreplan) find the
// assignment. Fixed pre-exams keep their slot; all non-fixed exams are re-planned
// (with keepAssigned, currently-slotted non-fixed exams are kept too). When no Anny
// rooms are booked anywhere, nothing is assigned.
func (p *Plexams) GeneratePreplanAssignment(ctx context.Context, keepAssigned bool) (*model.PreplanValidation, error) {
	preExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	if len(preExams) == 0 {
		return &model.PreplanValidation{Ok: true, Messages: []string{}, UnassignedIDs: []int{}}, nil
	}
	// candidate slots = ALL regular exam slots (not only the MUC.DAI slots): the
	// pre-exams go wherever we have booked Anny rooms, and those bookings sit on the
	// normal slot grid (08:30/10:30/12:30/14:30/16:30), not just the MUC.DAI pattern.
	regularSlots := p.semesterConfig.Slots
	if len(regularSlots) == 0 {
		return nil, fmt.Errorf("no slots configured for this semester")
	}
	exahmRooms, sebRooms, err := p.preplanRoomCapacities(ctx)
	if err != nil {
		return nil, err
	}
	// SEB exams that fit into a single R-building lab are "small": they are NOT placed
	// into the booked Anny slots, only flagged to be planned in the R-building.
	rBauSebThreshold, err := p.maxNonAnnySebRoom(ctx)
	if err != nil {
		return nil, err
	}

	// booked Anny capacity per regular slot
	allKeys := make([][2]int, 0, len(regularSlots))
	for _, s := range regularSlots {
		allKeys = append(allKeys, [2]int{s.DayNumber, s.SlotNumber})
	}
	booked, err := p.annyBookedBySlot(ctx, allKeys)
	if err != nil {
		return nil, err
	}

	// candidate slots: only those with booked Anny rooms; usable capacity = 90%
	slots := make([]*preplanSlot, 0)
	slotIdxByKey := make(map[[2]int]int)
	for _, s := range regularSlots {
		sb := booked[[2]int{s.DayNumber, s.SlotNumber}]
		if sb == nil {
			continue
		}
		capacity := int(float64(sb.seats) * preplanCapacityFactor)
		if capacity <= 0 {
			continue
		}
		slotIdxByKey[[2]int{s.DayNumber, s.SlotNumber}] = len(slots)
		slots = append(slots, &preplanSlot{day: s.DayNumber, slotNo: s.SlotNumber, capacity: capacity})
	}

	// MUC.DAI slots are reserved: an exam with a MUC.DAI program (DE/GS/ID) may ONLY be
	// placed in a (booked) MUC.DAI slot; all other exams may use any booked slot.
	mucdaiProgs := make(map[string]bool)
	for _, prog := range p.mucdaiProgramNames(ctx) {
		mucdaiProgs[prog] = true
	}
	mucdaiSlotIdx := make(map[int]bool)
	for _, s := range p.semesterConfig.MucDaiSlots {
		if idx, ok := slotIdxByKey[[2]int{s.DayNumber, s.SlotNumber}]; ok {
			mucdaiSlotIdx[idx] = true
		}
	}

	// same-slot groups → units (union-find over pre-exam indices)
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
	groupMembers := make(map[int][]int)
	groupOrder := make([]int, 0)
	for i := range preExams {
		r := find(i)
		if _, ok := groupMembers[r]; !ok {
			groupOrder = append(groupOrder, r)
		}
		groupMembers[r] = append(groupMembers[r], i)
	}

	// per pre-exam: the final slot (nil = none) and whether it stays fixed
	finalSlot := make([]*preplanSlot, len(preExams))
	finalFixed := make([]bool, len(preExams))

	// fixed/kept occupancy of the candidate slots, fed into the solver
	fixedUsed := make([]int, len(slots))
	fixedProgs := make([]map[string]bool, len(slots))
	for s := range fixedProgs {
		fixedProgs[s] = map[string]bool{}
	}

	solveUnits := make([]*preplanUnit, 0, len(groupOrder))
	solveMembers := make([][]int, 0, len(groupOrder)) // members per solve unit, by solve index
	smallSeb := make([]int, 0)                        // pre-exam indices to plan in the R-building

	for _, r := range groupOrder {
		members := groupMembers[r]
		seats, hasExahm, minID := 0, false, preExams[members[0]].ID
		programs := map[string]bool{}
		anyFixed := false
		var fixedKey *[2]int
		for _, i := range members {
			pe := preExams[i]
			seats += pe.ExpectedStudents
			if pe.ExamKind == "EXaHM" {
				hasExahm = true
			}
			for _, prog := range pe.Programs {
				programs[prog] = true
			}
			if pe.ID < minID {
				minID = pe.ID
			}
			if pe.IsFixed && pe.PlannedDayNumber != nil && pe.PlannedSlotNumber != nil {
				anyFixed = true
				k := [2]int{*pe.PlannedDayNumber, *pe.PlannedSlotNumber}
				fixedKey = &k
			}
		}

		// pin key: a fixed member's slot, or (keepAssigned) the common current slot
		var pinKey *[2]int
		fixed := false
		switch {
		case anyFixed:
			pinKey, fixed = fixedKey, true
		case keepAssigned:
			if k := commonSlotKey(preExams, members); k != nil {
				if _, ok := slotIdxByKey[*k]; ok {
					pinKey = k
				}
			}
		}

		if pinKey != nil {
			ps := &preplanSlot{day: pinKey[0], slotNo: pinKey[1]}
			if idx, ok := slotIdxByKey[*pinKey]; ok {
				ps = slots[idx]
				fixedUsed[idx] += seats
				for prog := range programs {
					fixedProgs[idx][prog] = true
				}
			}
			for _, i := range members {
				finalSlot[i] = ps
				finalFixed[i] = fixed
			}
			continue
		}

		// small SEB (fits a single R-building lab) → plan in the R-building, not Anny
		if !hasExahm && seats <= rBauSebThreshold {
			for _, i := range members {
				finalSlot[i] = nil
				finalFixed[i] = false
				smallSeb = append(smallSeb, i)
			}
			continue
		}

		dropCost := preplanDropBase + seats
		if hasExahm {
			dropCost += preplanExahmKeep
		}
		var allowedSlots map[int]bool
		for prog := range programs {
			if mucdaiProgs[prog] {
				allowedSlots = mucdaiSlotIdx // restrict MUC.DAI exams to MUC.DAI slots
				break
			}
		}
		solveUnits = append(solveUnits, &preplanUnit{
			members: members, seats: seats, programs: programs,
			hasExahm: hasExahm, dropCost: dropCost, minID: minID,
			allowedSlots: allowedSlots,
		})
		solveMembers = append(solveMembers, members)
	}

	// explicit "nicht gleichzeitig" pairs (PreplanExam.NotSameSlot) → strong conflicts
	unitOfExam := make(map[int]int, len(preExams))
	for ui, members := range solveMembers {
		for _, mi := range members {
			unitOfExam[preExams[mi].ID] = ui
		}
	}
	for _, pe := range preExams {
		ui, ok := unitOfExam[pe.ID]
		if !ok {
			continue
		}
		for _, otherID := range pe.NotSameSlot {
			uj, ok := unitOfExam[otherID]
			if !ok || uj == ui {
				continue
			}
			if solveUnits[ui].conflicts == nil {
				solveUnits[ui].conflicts = map[int]int{}
			}
			if solveUnits[uj].conflicts == nil {
				solveUnits[uj].conflicts = map[int]int{}
			}
			solveUnits[ui].conflicts[uj] = preplanExplicitConflictWeight
			solveUnits[uj].conflicts[ui] = preplanExplicitConflictWeight
		}
	}

	assign := solvePreplan(solveUnits, slots, fixedUsed, fixedProgs)
	for u, members := range solveMembers {
		var ps *preplanSlot
		if assign[u] >= 0 {
			ps = slots[assign[u]]
		}
		for _, i := range members {
			finalSlot[i] = ps
			finalFixed[i] = false
		}
	}

	// persist
	for i, pe := range preExams {
		if ps := finalSlot[i]; ps != nil {
			day, slot := ps.day, ps.slotNo
			pe.PlannedDayNumber = &day
			pe.PlannedSlotNumber = &slot
		} else {
			pe.PlannedDayNumber = nil
			pe.PlannedSlotNumber = nil
		}
		pe.IsFixed = finalFixed[i]
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

	// notes for the small SEB that are deliberately left out of the Anny planning
	smallByID := make(map[int]bool, len(smallSeb))
	for _, i := range smallSeb {
		pe := preExams[i]
		smallByID[pe.ID] = true
		result.Messages = append(result.Messages, fmt.Sprintf(
			"SEB %d (%s, %d Plätze): klein genug für den R-Bau (≤ %d) — dort einplanen, nicht in Anny",
			pe.ID, pe.Module, pe.ExpectedStudents, rBauSebThreshold))
	}

	// EXaHM / large SEB that could not be placed → not enough booked Anny capacity
	mustPlaceUnplaced := 0
	for _, id := range result.UnassignedIDs {
		if !smallByID[id] {
			mustPlaceUnplaced++
		}
	}
	if mustPlaceUnplaced > 0 {
		totalCap := 0
		for _, s := range slots {
			totalCap += s.capacity
		}
		result.Messages = append(result.Messages, fmt.Sprintf(
			"%d EXaHM/große SEB ohne Slot — gebuchte Anny-Plätze reichen nicht (nutzbar %d bei %d Slots); bitte mehr Anny-Slots buchen",
			mustPlaceUnplaced, totalCap, len(slots)))
	}

	if len(slots) == 0 {
		result.Messages = append([]string{"keine Anny-Räume gebucht — nichts zugeordnet (zuerst Anny-Räume buchen und importieren)"}, result.Messages...)
	}
	result.Ok = len(result.Messages) == 0
	return result, nil
}

// commonSlotKey returns the slot key shared by all members, or nil when they are not
// all in the same slot.
func commonSlotKey(preExams []*model.PreplanExam, members []int) *[2]int {
	var key *[2]int
	for _, i := range members {
		pe := preExams[i]
		if pe.PlannedDayNumber == nil || pe.PlannedSlotNumber == nil {
			return nil
		}
		k := [2]int{*pe.PlannedDayNumber, *pe.PlannedSlotNumber}
		if key == nil {
			key = &k
		} else if *key != k {
			return nil
		}
	}
	return key
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
