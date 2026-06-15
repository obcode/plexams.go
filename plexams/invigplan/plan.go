package invigplan

import "sort"

// Plan is the mutable assignment the optimizer searches over. Assign maps a
// position index to an invigilator id (or Unassigned); byInvig is the inverse
// index, kept in sync by Set/Clear.
type Plan struct {
	prob    *Problem
	Assign  []int
	byInvig map[int][]int
}

// NewPlan returns an empty plan with all fixed positions already applied.
func NewPlan(prob *Problem) *Plan {
	pl := &Plan{
		prob:    prob,
		Assign:  make([]int, len(prob.Positions)),
		byInvig: make(map[int][]int),
	}
	for i := range pl.Assign {
		pl.Assign[i] = Unassigned
	}
	for posIdx, invigID := range prob.Fixed {
		pl.set(posIdx, invigID)
	}
	return pl
}

// IsFixed reports whether a position is locked (pre-planned or self).
func (pl *Plan) IsFixed(posIdx int) bool {
	_, ok := pl.prob.Fixed[posIdx]
	return ok
}

// Set assigns a position to an invigilator. Fixed positions cannot be changed.
func (pl *Plan) Set(posIdx, invigID int) {
	if pl.IsFixed(posIdx) {
		return
	}
	pl.set(posIdx, invigID)
}

func (pl *Plan) set(posIdx, invigID int) {
	pl.clear(posIdx)
	pl.Assign[posIdx] = invigID
	idx := sort.SearchInts(pl.byInvig[invigID], posIdx)
	pl.byInvig[invigID] = append(pl.byInvig[invigID], 0)
	copy(pl.byInvig[invigID][idx+1:], pl.byInvig[invigID][idx:])
	pl.byInvig[invigID][idx] = posIdx
}

// Clear unassigns a position. Fixed positions are left untouched.
func (pl *Plan) Clear(posIdx int) {
	if pl.IsFixed(posIdx) {
		return
	}
	pl.clear(posIdx)
}

func (pl *Plan) clear(posIdx int) {
	old := pl.Assign[posIdx]
	if old == Unassigned {
		return
	}
	positions := pl.byInvig[old]
	idx := sort.SearchInts(positions, posIdx)
	if idx < len(positions) && positions[idx] == posIdx {
		pl.byInvig[old] = append(positions[:idx], positions[idx+1:]...)
	}
	pl.Assign[posIdx] = Unassigned
}

// Positions returns the sorted position indices assigned to an invigilator.
func (pl *Plan) Positions(invigID int) []int {
	return pl.byInvig[invigID]
}

// HasInSlot reports whether the invigilator already has a position in (day,slot),
// ignoring the position exceptPos (use -1 to ignore none).
func (pl *Plan) HasInSlot(invigID, day, slot, exceptPos int) bool {
	for _, posIdx := range pl.byInvig[invigID] {
		if posIdx == exceptPos {
			continue
		}
		pos := pl.prob.Positions[posIdx]
		if pos.Day == day && pos.Slot == slot {
			return true
		}
	}
	return false
}

// DoingMinutes returns the minutes assigned to an invigilator that count toward
// the contingent (self-invigilations contribute 0).
func (pl *Plan) DoingMinutes(invigID int) int {
	sum := 0
	for _, posIdx := range pl.byInvig[invigID] {
		sum += pl.prob.Positions[posIdx].Minutes
	}
	return sum
}

// CountKind returns how many positions of the given kind are assigned to the
// invigilator.
func (pl *Plan) CountKind(invigID int, kind Kind) int {
	n := 0
	for _, posIdx := range pl.byInvig[invigID] {
		if pl.prob.Positions[posIdx].Kind() == kind {
			n++
		}
	}
	return n
}

// Days returns the distinct day numbers the invigilator is assigned to.
func (pl *Plan) Days(invigID int) []int {
	seen := make(map[int]bool)
	for _, posIdx := range pl.byInvig[invigID] {
		seen[pl.prob.Positions[posIdx].Day] = true
	}
	days := make([]int, 0, len(seen))
	for d := range seen {
		days = append(days, d)
	}
	sort.Ints(days)
	return days
}

// Unfilled returns the indices of positions that still have no invigilator.
func (pl *Plan) Unfilled() []int {
	var open []int
	for posIdx, invigID := range pl.Assign {
		if invigID == Unassigned {
			open = append(open, posIdx)
		}
	}
	return open
}

// Clone returns a deep copy of the plan sharing the same Problem.
func (pl *Plan) Clone() *Plan {
	cp := &Plan{
		prob:    pl.prob,
		Assign:  make([]int, len(pl.Assign)),
		byInvig: make(map[int][]int, len(pl.byInvig)),
	}
	copy(cp.Assign, pl.Assign)
	for id, positions := range pl.byInvig {
		cp.byInvig[id] = append([]int(nil), positions...)
	}
	return cp
}
