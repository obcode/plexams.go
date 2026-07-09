package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/preplancalc"
)

// preplanCapacityFactor is the usable fraction of a slot's booked Anny seats. 1.0 =
// fill the booked rooms completely (no reserve).
const preplanCapacityFactor = 1.0

// weekdaysDE maps time.Weekday (Sunday=0) to the German two-letter abbreviation.
var weekdaysDE = [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

// slotLabelDE renders a slot start time as e.g. "Do 16.07. 10:30" with the correct German
// weekday. (Go's reference layout uses "Mon"/"Monday"; a literal "Mo" would print every day
// as Monday.)
func slotLabelDE(t time.Time) string {
	return weekdaysDE[int(t.Weekday())] + " " + t.Format("02.01. 15:04")
}

// ValidatePreplanAssignment checks the current slot assignment of the pre-exams:
// unassigned exams, per-slot/kind capacity overflow, and study programs shared by
// more than one exam in the same slot. ok is true when there are no findings.
func (p *Plexams) ValidatePreplanAssignment(ctx context.Context) (*model.PreplanValidation, error) {
	preExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	// No SEB/EXaHM pre-exams yet → nothing to validate. Report as skipped (neutral),
	// not as a green pass, so the GUI shows "übersprungen".
	if len(preExams) == 0 {
		return skippedPreplanValidation(), nil
	}
	exahmRooms, sebRooms, err := p.preplanRoomCapacities(ctx)
	if err != nil {
		return nil, err
	}
	rBauSebThreshold, err := p.nonAnnySebSeats(ctx)
	if err != nil {
		return nil, err
	}

	starts := make([]time.Time, 0)
	for _, pe := range preExams {
		if pe.PlannedStarttime != nil {
			starts = append(starts, *pe.PlannedStarttime)
		}
	}
	booked, err := p.annyBookedByTime(ctx, starts)
	if err != nil {
		return nil, err
	}
	exahmIntervals, err := p.bookedExahmIntervals(ctx)
	if err != nil {
		return nil, err
	}
	blockDur := slotBlockDuration(p.semesterConfig.Starttimes)

	return validatePreplan(preExams, exahmRooms, sebRooms, booked, rBauSebThreshold, exahmIntervals, blockDur), nil
}

// skippedPreplanValidation is the neutral result returned when there are no SEB/EXaHM
// pre-exams to validate/assign yet. Skipped is true so the GUI renders "übersprungen"
// instead of a misleading green pass.
func skippedPreplanValidation() *model.PreplanValidation {
	reason := skipNoPreExams
	return &model.PreplanValidation{
		Ok:            true,
		Skipped:       true,
		SkipReason:    &reason,
		Messages:      []string{},
		UnassignedIDs: []int{},
		Findings:      []*model.PreplanFinding{},
	}
}

// validatePreplan builds the validation result from an in-memory set of pre-exams.
// booked (Anny bookings per slot) may be nil; when present, missing bookings are
// reported so the planner can book step by step. SEB exams that fit a single R-building
// lab (seats <= rBauSebThreshold) are reported as "plan in the R-building" instead of
// being counted as genuinely unplaced.
func validatePreplan(preExams []*model.PreplanExam, exahmRooms, sebRooms []preplancalc.RoomCapacity, booked map[time.Time]*slotBooking, rBauSebThreshold int, exahmIntervals []bookedRoomInterval, blockDur time.Duration) *model.PreplanValidation {
	findings := make([]*model.PreplanFinding, 0)
	addFinding := func(level model.ValidationLevel, format string, a ...any) {
		findings = append(findings, &model.PreplanFinding{Level: level, Message: fmt.Sprintf(format, a...)})
	}

	unassigned := make([]int, 0)
	genuineUnassigned := 0
	// SEB exams that fit the R-building labs (split across them) need no Anny booking at all,
	// so leaving them unassigned is by design, not a failure — grade them as warnings.
	smallSebNotes := make([]*model.PreplanFinding, 0)
	bySlot := make(map[time.Time][]*model.PreplanExam)
	slotOrder := make([]time.Time, 0)
	for _, pe := range preExams {
		if pe.PlannedStarttime == nil {
			unassigned = append(unassigned, pe.ID)
			if pe.ExamKind == "SEB" && pe.ExpectedStudents <= rBauSebThreshold {
				smallSebNotes = append(smallSebNotes, &model.PreplanFinding{
					Level: model.ValidationLevelWarning,
					Message: fmt.Sprintf(
						"SEB %s (%d Plätze): passt in den R-Bau (≤ %d Plätze gesamt) — dort einplanen, kein Anny-Slot nötig",
						pe.Module, pe.ExpectedStudents, rBauSebThreshold),
				})
			} else {
				genuineUnassigned++
			}
			continue
		}
		key := *pe.PlannedStarttime
		if _, ok := bySlot[key]; !ok {
			slotOrder = append(slotOrder, key)
		}
		bySlot[key] = append(bySlot[key], pe)
	}
	sort.Slice(slotOrder, func(i, j int) bool { return slotOrder[i].Before(slotOrder[j]) })

	if genuineUnassigned > 0 {
		addFinding(model.ValidationLevelError, "%s ohne Slot — gebuchte Anny-Plätze reichen nicht, bitte mehr Anny-Slots buchen",
			pluralN(genuineUnassigned, "Prüfung", "Prüfungen"))
	}
	findings = append(findings, smallSebNotes...)

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
			// an EXaHM/SEB exam placed here needs booked rooms whose Anny window is long
			// enough for its real exam window (duration + setup/teardown buffer) and provide
			// enough seats. Flag a placement no booking can cover (e.g. Embedded Computing at
			// 16:30 with a 60/60 buffer while the room is only booked 16:00–18:30, or a SEB in
			// a 10:30 slot whose rooms are booked only until 11:30). EXaHM needs EXaHM rooms;
			// SEB accepts EXaHM or SEB rooms.
			if pe.ExamKind == "EXaHM" || pe.ExamKind == "SEB" {
				isExahm := pe.ExamKind == "EXaHM"
				dur := preplanExamDuration(pe, blockDur)
				pre, post := exahmRoomBuffers(pe.Constraints)
				if seats := exahmWindowSeats(exahmIntervals, isExahm, key, dur, pre, post); seats < pe.ExpectedStudents {
					addFinding(model.ValidationLevelError,
						"Slot %s: %s %s braucht die Räume %s–%s für %d Plätze (%d min + %d/%d min Puffer), aber gebuchte %s-Räume, die dieses Fenster abdecken, bieten nur %d Plätze",
						slotLabelDE(key), pe.ExamKind, pe.Module,
						key.Add(-pre).Format("15:04"), key.Add(dur+post).Format("15:04"),
						pe.ExpectedStudents, int(dur.Minutes()), int(pre.Minutes()), int(post.Minutes()), pe.ExamKind, seats)
				}
			}
		}

		var sb *slotBooking
		if booked != nil {
			sb = booked[key]
		}
		// honour per-exam room restrictions for the suggestion/capacity
		exahmPool := preplancalc.RoomsForKind(exams, "EXaHM", exahmRooms)
		sebPool := preplancalc.RoomsForKind(exams, "SEB", sebRooms)
		findings = append(findings, kindBookingFindings(key, "EXaHM", exahm, preplancalc.TotalSeats(exahmPool), exahmPool, sb)...)
		findings = append(findings, kindBookingFindings(key, "SEB", seb, preplancalc.TotalSeats(sebPool), sebPool, sb)...)
		// note: same study program in one slot is allowed (soft spreading only), so it
		// is no longer reported as a finding here.
	}

	// Capacity over time: the simultaneously-occupied EXaHM/total booked seats (exam time
	// plus setup/teardown) must stay within the booking at every instant. Catches a long or
	// override exam whose setup reaches back into the previous slot (Embedded Computing needs
	// its rooms from 09:30 while the 08:30 exams still hold them until 10:15) — which the
	// per-slot seat sums miss. Rooms may be shared, so this is an aggregate seat count.
	{
		cumExams := make([]cumExam, 0, len(preExams))
		moduleByID := make(map[int]string, len(preExams))
		for _, pe := range preExams {
			if pe.PlannedStarttime == nil {
				continue
			}
			moduleByID[pe.ID] = pe.Module
			occPre, occPost := exahmOccBuffers(pe.Constraints)
			st := *pe.PlannedStarttime
			cumExams = append(cumExams, cumExam{
				id: pe.ID, seats: pe.ExpectedStudents, exahm: pe.ExamKind == "EXaHM",
				from: st.Add(-occPre), to: st.Add(preplanExamDuration(pe, blockDur) + occPost),
			})
		}
		// merge contiguous overload intervals for readable messages
		merged := make([]cumOverload, 0)
		for _, ov := range cumulativeOverloads(cumExams, exahmIntervals, false) {
			if k := len(merged) - 1; k >= 0 && merged[k].exahm == ov.exahm && !ov.from.After(merged[k].to) {
				if ov.to.After(merged[k].to) {
					merged[k].to = ov.to
				}
				if ov.demand > merged[k].demand {
					merged[k].demand = ov.demand
				}
				seen := make(map[int]bool)
				for _, id := range merged[k].examIDs {
					seen[id] = true
				}
				for _, id := range ov.examIDs {
					if !seen[id] {
						merged[k].examIDs = append(merged[k].examIDs, id)
					}
				}
				continue
			}
			merged = append(merged, ov)
		}
		for _, ov := range merged {
			kind := "EXaHM"
			if !ov.exahm {
				kind = "EXaHM+SEB"
			}
			mods := make([]string, 0, len(ov.examIDs))
			seen := make(map[string]bool)
			for _, id := range ov.examIDs {
				if m := moduleByID[id]; m != "" && !seen[m] {
					mods = append(mods, m)
					seen[m] = true
				}
			}
			addFinding(model.ValidationLevelError,
				"%s %s–%s: %s %d Plätze gleichzeitig belegt, nur %d gebucht — überlappende Vor-/Nachläufe (%s)",
				weekdaysDE[int(ov.from.Weekday())], ov.from.Format("02.01. 15:04"), ov.to.Format("15:04"),
				kind, ov.demand, ov.seats, joinStrings(mods))
		}
	}

	assignedCount := len(preExams) - len(unassigned)
	messages := make([]string, len(findings))
	ok := true
	for i, f := range findings {
		messages[i] = f.Message
		if f.Level == model.ValidationLevelError {
			ok = false
		}
	}
	return &model.PreplanValidation{
		Ok:            ok,
		AssignedCount: assignedCount,
		UnassignedIDs: unassigned,
		Messages:      messages,
		Findings:      findings,
	}
}

// kindBookingFindings reports, for one slot and kind, a physical-capacity overflow
// (can't fit even fully booked) or — when within capacity — the Anny bookings still
// missing to cover the demand. Both are errors: the slot cannot host the exams as
// booked.
func kindBookingFindings(key time.Time, kind string, needed, available int, rooms []preplancalc.RoomCapacity, sb *slotBooking) []*model.PreplanFinding {
	if needed == 0 {
		return nil
	}
	slotLabel := slotLabelDE(key)
	if needed > available {
		return []*model.PreplanFinding{{
			Level: model.ValidationLevelError,
			Message: fmt.Sprintf("Slot %s: %s %d Plätze nötig, nur %d verfügbar (Kapazität reicht nicht)",
				slotLabel, kind, needed, available),
		}}
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

	toBook := preplancalc.RoomsToBook(rooms, needed-bookedSeats, bookedRooms)
	msg := fmt.Sprintf("Slot %s: %s noch %d Plätze zu buchen (gebucht %d von %d nötig)",
		slotLabel, kind, needed-bookedSeats, bookedSeats, needed)
	if len(toBook) > 0 {
		msg += " — z. B. " + joinStrings(toBook)
	}
	return []*model.PreplanFinding{{Level: model.ValidationLevelError, Message: msg}}
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
		return skippedPreplanValidation(), nil
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
	rBauSebThreshold, err := p.nonAnnySebSeats(ctx)
	if err != nil {
		return nil, err
	}

	// booked Anny capacity per regular slot
	allStarts := make([]time.Time, 0, len(regularSlots))
	for _, s := range regularSlots {
		allStarts = append(allStarts, s.Starttime)
	}
	booked, err := p.annyBookedByTime(ctx, allStarts)
	if err != nil {
		return nil, err
	}
	// booked T-building rooms as time intervals, to gate each EXaHM exam to slots where a
	// booked EXaHM room actually covers its real window (duration + setup/teardown buffer),
	// not merely to slots that have some booked seats.
	exahmIntervals, err := p.bookedExahmIntervals(ctx)
	if err != nil {
		return nil, err
	}
	blockDur := slotBlockDuration(p.semesterConfig.Starttimes)

	// candidate slots: only those with booked Anny rooms; usable capacity = 90%
	slots := make([]*preplanSlot, 0)
	slotIdxByStart := make(map[time.Time]int)
	// usable fraction of each slot's booked Anny seats (GUI-editable, default 1.0)
	capacityFactor := preplanCapacityFactor
	if cfg, err := p.GenerationConfig(ctx); err == nil && cfg.PreplanCapacityFactor > 0 {
		capacityFactor = cfg.PreplanCapacityFactor
	}
	for _, s := range regularSlots {
		sb := booked[s.Starttime]
		if sb == nil {
			continue
		}
		capacity := int(float64(sb.seats) * capacityFactor)
		if capacity <= 0 {
			continue
		}
		slotIdxByStart[s.Starttime] = len(slots)
		slots = append(slots, &preplanSlot{start: s.Starttime, capacity: capacity})
	}

	// MUC.DAI slots are reserved: an exam with a MUC.DAI program (DE/GS/ID) may ONLY be
	// placed in a (booked) MUC.DAI slot; all other exams may use any booked slot.
	mucdaiProgs := make(map[string]bool)
	for _, prog := range p.mucdaiProgramNames(ctx) {
		mucdaiProgs[prog] = true
	}
	mucdaiSlotIdx := make(map[int]bool)
	for _, s := range p.semesterConfig.MucDaiSlots {
		if idx, ok := slotIdxByStart[s.Starttime]; ok {
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

	for _, r := range groupOrder {
		members := groupMembers[r]
		seats, hasExahm, minID := 0, false, preExams[members[0]].ID
		programs := map[string]bool{}
		anyFixed := false
		var fixedKey *time.Time
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
			if pe.IsFixed && pe.PlannedStarttime != nil {
				anyFixed = true
				k := *pe.PlannedStarttime
				fixedKey = &k
			}
		}

		// pin key: a fixed member's slot, or (keepAssigned) the common current slot
		var pinKey *time.Time
		fixed := false
		switch {
		case anyFixed:
			pinKey, fixed = fixedKey, true
		case keepAssigned:
			if k := commonSlotKey(preExams, members); k != nil {
				if _, ok := slotIdxByStart[*k]; ok {
					pinKey = k
				}
			}
		}

		if pinKey != nil {
			ps := &preplanSlot{start: *pinKey}
			if idx, ok := slotIdxByStart[*pinKey]; ok {
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

		// A SEB that fits the R-building labs (can be split across them) does not NEED an Anny
		// slot: it fills leftover Anny capacity if some is free, otherwise it is left for the
		// R-building (only a warning). Its drop cost grows with the SQUARE of its size, so when
		// Anny is tight the solver keeps the LARGE (and same-slot-coupled, hence large) exams
		// placed and instead drops several SMALLER independent ones — dropping one big coupled
		// unit is dearer than dropping a few small ones that can each sit alone in the R-building.
		// Capped below preplanDropBase so an R-building SEB never outranks a must-place exam.
		rBauSeb := !hasExahm && seats <= rBauSebThreshold

		dropCost := preplanDropBase + seats
		switch {
		case hasExahm:
			dropCost += preplanExahmKeep
		case rBauSeb:
			dropCost = preplanSmallSebDrop + seats*seats
			if dropCost >= preplanDropBase {
				dropCost = preplanDropBase - 1
			}
		}
		var allowedSlots map[int]bool
		for prog := range programs {
			if mucdaiProgs[prog] {
				allowedSlots = mucdaiSlotIdx // restrict MUC.DAI exams to MUC.DAI slots
				break
			}
		}
		// EXaHM and SEB pre-exams run in booked T-building rooms. Compute the unit's window
		// (max exam duration + setup/teardown buffer over its members) and restrict it to
		// candidate slots where booked rooms of the right kind cover that window with enough
		// seats — rooms booked too short do not count. EXaHM needs EXaHM rooms; SEB accepts
		// EXaHM or SEB rooms. An empty set leaves the unit unplaced (small SEB then fall back
		// to the R-building, others are reported by validatePreplan).
		var uDur, uPre, uPost, uOccPre, uOccPost time.Duration
		for _, i := range members {
			pe := preExams[i]
			if d := preplanExamDuration(pe, blockDur); d > uDur {
				uDur = d
			}
			pr, po := exahmRoomBuffers(pe.Constraints)
			if pr > uPre {
				uPre = pr
			}
			if po > uPost {
				uPost = po
			}
			opr, opo := exahmOccBuffers(pe.Constraints)
			if opr > uOccPre {
				uOccPre = opr
			}
			if opo > uOccPost {
				uOccPost = opo
			}
		}
		windowOK := make(map[int]bool)
		for idx, ps := range slots {
			if exahmWindowSeats(exahmIntervals, hasExahm, ps.start, uDur, uPre, uPost) >= seats {
				windowOK[idx] = true
			}
		}
		allowedSlots = intersectSlotSet(allowedSlots, windowOK)

		solveUnits = append(solveUnits, &preplanUnit{
			members: members, seats: seats, programs: programs,
			hasExahm: hasExahm, dropCost: dropCost, minID: minID,
			allowedSlots: allowedSlots,
			dur:          uDur, occPre: uOccPre, occPost: uOccPost,
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

	// explicit "darf zusammen / direkt nacheinander" pairs (PreplanExam.CanShareSlot)
	// → exempt that pair from the program-based spreading
	for _, pe := range preExams {
		ui, ok := unitOfExam[pe.ID]
		if !ok {
			continue
		}
		for _, otherID := range pe.CanShareSlot {
			uj, ok := unitOfExam[otherID]
			if !ok || uj == ui {
				continue
			}
			if solveUnits[ui].compatible == nil {
				solveUnits[ui].compatible = map[int]bool{}
			}
			if solveUnits[uj].compatible == nil {
				solveUnits[uj].compatible = map[int]bool{}
			}
			solveUnits[ui].compatible[uj] = true
			solveUnits[uj].compatible[ui] = true
		}
	}

	assign := solvePreplan(solveUnits, slots, fixedUsed, fixedProgs, exahmIntervals)
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
			start := ps.start
			pe.PlannedStarttime = &start
		} else {
			pe.PlannedStarttime = nil
		}
		pe.IsFixed = finalFixed[i]
		if _, err := p.dbClient.ReplacePreplanExam(ctx, pe); err != nil {
			return nil, err
		}
	}

	starts := make([]time.Time, 0)
	for _, pe := range preExams {
		if pe.PlannedStarttime != nil {
			starts = append(starts, *pe.PlannedStarttime)
		}
	}
	bookedAfter, err := p.annyBookedByTime(ctx, starts)
	if err != nil {
		return nil, err
	}
	// validatePreplan reports the small-SEB R-building notes and the genuinely-unplaced
	// must-place exams (threshold-aware), so no extra messages are added here.
	result := validatePreplan(preExams, exahmRooms, sebRooms, bookedAfter, rBauSebThreshold, exahmIntervals, blockDur)
	if len(slots) == 0 {
		msg := "keine Anny-Räume gebucht — nichts zugeordnet (zuerst Anny-Räume buchen und importieren)"
		result.Findings = append([]*model.PreplanFinding{{Level: model.ValidationLevelError, Message: msg}}, result.Findings...)
		result.Messages = append([]string{msg}, result.Messages...)
		result.Ok = false
	}
	return result, nil
}

// commonSlotKey returns the start time shared by all members, or nil when they are not
// all at the same start time.
func commonSlotKey(preExams []*model.PreplanExam, members []int) *time.Time {
	var key *time.Time
	for _, i := range members {
		pe := preExams[i]
		if pe.PlannedStarttime == nil {
			return nil
		}
		k := *pe.PlannedStarttime
		if key == nil {
			key = &k
		} else if !key.Equal(k) {
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
