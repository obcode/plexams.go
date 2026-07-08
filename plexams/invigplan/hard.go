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
	return in.Available(pos)
}

func (c availabilityHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	for posIdx, invigID := range plan.Assign {
		if invigID == Unassigned {
			continue
		}
		in := p.Invigilator(invigID)
		pos := p.Positions[posIdx]
		if in == nil || !in.Available(pos) {
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: invigID,
				Start:         pos.Start,
				Message:       fmt.Sprintf("invigilator %d assigned to unavailable slot %s", invigID, pos.Start.Format("02.01. 15:04")),
			})
		}
	}
	return vs
}

// timeWindowHard: a person may only take an invigilation that fits the
// per-date time windows they configured (semester.yaml timeWindows). The check
// uses the position's real start and end time, so an invigilation running long
// because of an NTA extension is rejected if it would finish after the allowed
// "until" – even when a normal-length invigilation in the same slot still fits.
type timeWindowHard struct{}

func (timeWindowHard) Name() string { return "time-window" }

func (timeWindowHard) Allows(p *Problem, plan *Plan, posIdx, invigID int) bool {
	in := p.Invigilator(invigID)
	if in == nil {
		return false
	}
	return in.AllowsTime(p.Positions[posIdx])
}

func (c timeWindowHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	for posIdx, invigID := range plan.Assign {
		if invigID == Unassigned {
			continue
		}
		in := p.Invigilator(invigID)
		pos := p.Positions[posIdx]
		if in == nil || in.AllowsTime(pos) {
			continue
		}
		room := pos.Room
		if room == "" {
			room = "reserve"
		}
		vs = append(vs, Violation{
			Constraint:    c.Name(),
			InvigilatorID: invigID,
			Start:         pos.Start,
			Message: fmt.Sprintf("invigilator %d assigned to %s at %s running %s-%s, outside their allowed time window",
				invigID, room, pos.Start.Format("02.01. 15:04"),
				pos.Start.Format("15:04"), pos.End().Format("15:04")),
		})
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
				Start:         pos.Start,
				Message:       fmt.Sprintf("invigilator %d has own exam at %s", invigID, pos.Start.Format("02.01. 15:04")),
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
	return !plan.HasAtTime(invigID, pos.Start.Unix(), posIdx)
}

func (c oneInvigilationPerSlotHard) Check(p *Problem, plan *Plan) []Violation {
	var vs []Violation
	type key struct {
		invigID int
		start   int64
	}
	seen := make(map[key]bool)
	for posIdx, invigID := range plan.Assign {
		if invigID == Unassigned {
			continue
		}
		pos := p.Positions[posIdx]
		k := key{invigID, pos.Start.Unix()}
		if seen[k] {
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: invigID,
				Start:         pos.Start,
				Message:       fmt.Sprintf("invigilator %d has more than one invigilation at %s", invigID, pos.Start.Format("02.01. 15:04")),
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
						Start:         a.Start,
						Message: fmt.Sprintf("invigilator %d: less than %d min between %s and %s",
							invigID, p.TimelagMin, a.Start.Format("02.01. 15:04"), b.Start.Format("02.01. 15:04")),
					})
				}
			}
		}
	}
	return vs
}

// gapOK reports whether two positions are far enough apart in time. Positions on
// different calendar days never conflict.
func gapOK(a, b Position, lag time.Duration) bool {
	if dateKey(a.Start) != dateKey(b.Start) {
		return true
	}
	earlier, later := a, b
	if later.Start.Before(earlier.Start) {
		earlier, later = b, a
	}
	return !later.Start.Before(earlier.End().Add(lag))
}
