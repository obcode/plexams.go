package invigplan

import (
	"fmt"
	"time"
)

// availabilityHard: no invigilation on an excluded day/slot, and – if the
// person restricted themselves – only in their allowed slots.
type availabilityHard struct{}

func (availabilityHard) Name() string { return "availability" }

func (availabilityHard) Allows(p *Problem, plan *Plan, posIdx, invigID int) bool {
	in := p.Invigilator(invigID)
	if in == nil {
		return false
	}
	pos := p.Positions[posIdx]
	return in.Available(pos.Day, pos.Slot)
}

func (c availabilityHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	for posIdx, invigID := range plan.Assign {
		if invigID == Unassigned {
			continue
		}
		in := p.Invigilator(invigID)
		pos := p.Positions[posIdx]
		if in == nil || !in.Available(pos.Day, pos.Slot) {
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: invigID,
				Day:           pos.Day,
				Slot:          pos.Slot,
				Message:       fmt.Sprintf("invigilator %d assigned to unavailable slot (%d,%d)", invigID, pos.Day, pos.Slot),
			})
		}
	}
	return vs
}

// ownExamHard: a person must not take a (non-self) invigilation in a slot in
// which they have an own exam. For NTA exams the builder extends OwnExamSlots to
// the following slot, so this also covers the "whole time during NTA" rule.
type ownExamHard struct{}

func (ownExamHard) Name() string { return "own-exam" }

func (ownExamHard) Allows(p *Problem, plan *Plan, posIdx, invigID int) bool {
	in := p.Invigilator(invigID)
	if in == nil {
		return false
	}
	return !in.OwnExamSlots[p.Positions[posIdx].SlotKey()]
}

func (c ownExamHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	for posIdx, invigID := range plan.Assign {
		if invigID == Unassigned {
			continue
		}
		pos := p.Positions[posIdx]
		if pos.IsSelf { // self-invigilation is the allowed case
			continue
		}
		in := p.Invigilator(invigID)
		if in != nil && in.OwnExamSlots[pos.SlotKey()] {
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: invigID,
				Day:           pos.Day,
				Slot:          pos.Slot,
				Message:       fmt.Sprintf("invigilator %d has own exam in slot (%d,%d)", invigID, pos.Day, pos.Slot),
			})
		}
	}
	return vs
}

// oneInvigilationPerSlotHard: a person can be in at most one place per slot.
type oneInvigilationPerSlotHard struct{}

func (oneInvigilationPerSlotHard) Name() string { return "one-per-slot" }

func (oneInvigilationPerSlotHard) Allows(p *Problem, plan *Plan, posIdx, invigID int) bool {
	pos := p.Positions[posIdx]
	return !plan.HasInSlot(invigID, pos.Day, pos.Slot, posIdx)
}

func (c oneInvigilationPerSlotHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	type key struct {
		invigID, day, slot int
	}
	seen := make(map[key]bool)
	for posIdx, invigID := range plan.Assign {
		if invigID == Unassigned {
			continue
		}
		pos := p.Positions[posIdx]
		k := key{invigID, pos.Day, pos.Slot}
		if seen[k] {
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: invigID,
				Day:           pos.Day,
				Slot:          pos.Slot,
				Message:       fmt.Sprintf("invigilator %d has more than one invigilation in slot (%d,%d)", invigID, pos.Day, pos.Slot),
			})
		}
		seen[k] = true
	}
	return vs
}

// timeGapHard: two invigilations of the same person need at least TimelagMin
// minutes between the end of the earlier and the start of the later one.
type timeGapHard struct{}

func (timeGapHard) Name() string { return "time-gap" }

func (timeGapHard) Allows(p *Problem, plan *Plan, posIdx, invigID int) bool {
	cand := p.Positions[posIdx]
	lag := time.Duration(p.TimelagMin) * time.Minute
	for _, other := range plan.Positions(invigID) {
		if other == posIdx {
			continue
		}
		op := p.Positions[other]
		if !gapOK(cand, op, lag) {
			return false
		}
	}
	return true
}

func (c timeGapHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	lag := time.Duration(p.TimelagMin) * time.Minute
	for invigID, positions := range plan.byInvig {
		for i := 0; i < len(positions); i++ {
			for j := i + 1; j < len(positions); j++ {
				a, b := p.Positions[positions[i]], p.Positions[positions[j]]
				if !gapOK(a, b, lag) {
					vs = append(vs, Violation{
						Constraint:    c.Name(),
						InvigilatorID: invigID,
						Day:           a.Day,
						Message: fmt.Sprintf("invigilator %d: less than %d min between (%d,%d) and (%d,%d)",
							invigID, p.TimelagMin, a.Day, a.Slot, b.Day, b.Slot),
					})
				}
			}
		}
	}
	return vs
}

// gapOK reports whether two positions are far enough apart in time. Positions on
// different days never conflict.
func gapOK(a, b Position, lag time.Duration) bool {
	if a.Day != b.Day {
		return true
	}
	earlier, later := a, b
	if later.Start.Before(earlier.Start) {
		earlier, later = b, a
	}
	return !later.Start.Before(earlier.End().Add(lag))
}
