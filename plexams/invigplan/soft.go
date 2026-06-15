package invigplan

import (
	"fmt"
	"time"
)

// minuteBalanceSoft is the primary objective: every invigilator's assigned
// minutes should be close to their target. The cost is the squared deviation
// (so it keeps improving), a violation is reported only once the deviation
// exceeds the tolerance (which is also the optimizer's stopping criterion).
type minuteBalanceSoft struct{}

func (minuteBalanceSoft) Name() string { return "minute-balance" }

func (c minuteBalanceSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	var within, beyond float64
	var vs []Violation
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		doing := plan.DoingMinutes(in.ID)
		dev := doing - in.TargetMinutes
		a := abs(dev)
		if a > p.ToleranceMin {
			// Dominant linear penalty for the minutes outside the band; the
			// within-band part is capped so the gentle term stays bounded.
			beyond += float64(a - p.ToleranceMin)
			within += float64(p.ToleranceMin * p.ToleranceMin)
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: in.ID,
				Penalty:       p.Weights.BeyondTolerance * float64(a-p.ToleranceMin),
				Message: fmt.Sprintf("invigilator %d is %d min off target (%d/%d, tolerance %d)",
					in.ID, dev, doing, in.TargetMinutes, p.ToleranceMin),
			})
		} else {
			within += float64(dev * dev)
		}
	}
	return p.Weights.MinuteBalance*within + p.Weights.BeyondTolerance*beyond, vs
}

// coverageSoft penalizes every position that has no invigilator. It is weighted
// very high so the optimizer fills everything it feasibly can.
type coverageSoft struct{}

func (coverageSoft) Name() string { return "coverage" }

func (c coverageSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	open := plan.Unfilled()
	vs := make([]Violation, 0, len(open))
	for _, posIdx := range open {
		pos := p.Positions[posIdx]
		room := pos.Room
		if pos.IsReserve {
			room = "reserve"
		}
		vs = append(vs, Violation{
			Constraint: c.Name(),
			Day:        pos.Day,
			Slot:       pos.Slot,
			Penalty:    p.Weights.Coverage,
			Message:    fmt.Sprintf("no invigilator for %s in slot (%d,%d)", room, pos.Day, pos.Slot),
		})
	}
	return p.Weights.Coverage * float64(len(open)), vs
}

// maxDaysSoft: a person should invigilate on at most 3 days when they have own
// exams on at most 3 days.
type maxDaysSoft struct{}

func (maxDaysSoft) Name() string { return "max-days" }

const maxInvigilationDays = 3

func (c maxDaysSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	var penalty float64
	var vs []Violation
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		if len(in.OwnExamDays) > maxInvigilationDays {
			continue
		}
		days := len(plan.Days(in.ID))
		if days > maxInvigilationDays {
			excess := days - maxInvigilationDays
			pen := p.Weights.MaxDays * float64(excess*excess)
			penalty += pen
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: in.ID,
				Penalty:       pen,
				Message:       fmt.Sprintf("invigilator %d invigilates on %d days (max %d)", in.ID, days, maxInvigilationDays),
			})
		}
	}
	return penalty, vs
}

// preferOwnExamDaysSoft nudges people who have own exams toward invigilating on
// days they are at the university anyway.
type preferOwnExamDaysSoft struct{}

func (preferOwnExamDaysSoft) Name() string { return "prefer-own-exam-days" }

func (c preferOwnExamDaysSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	var penalty float64
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		if len(in.OwnExamDays) == 0 {
			continue
		}
		extra := 0
		for _, day := range plan.Days(in.ID) {
			if !in.OwnExamDays[day] {
				extra++
			}
		}
		penalty += p.Weights.PreferExamDays * float64(extra)
	}
	return penalty, nil
}

// distributionSoft spreads positions of one kind (reserve or NTA) evenly across
// invigilators: minimizing the sum of squared per-person counts equalizes them
// for a fixed total. Pre-planned imbalances stay (they are fixed) but are not
// made worse.
type distributionSoft struct {
	kind Kind
}

func (c distributionSoft) Name() string { return "distribution-" + c.kind.String() }

func (c distributionSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	counts := make(map[int]int, len(p.Invigilators))
	total := 0
	for i := range p.Invigilators {
		n := plan.CountKind(p.Invigilators[i].ID, c.kind)
		counts[p.Invigilators[i].ID] = n
		total += n
	}
	if total == 0 {
		return 0, nil
	}
	mean := float64(total) / float64(len(p.Invigilators))

	var sumSquares float64
	var vs []Violation
	for id, n := range counts {
		sumSquares += float64(n * n)
		if float64(n) > mean+1.0 {
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: id,
				Message:       fmt.Sprintf("invigilator %d has %d %s (mean %.1f)", id, n, c.kind, mean),
			})
		}
	}
	return p.Weights.Distribution * sumSquares, vs
}

// daySpanSoft keeps the time from the first start to the last end on a single
// day within MaxSpanHours.
type daySpanSoft struct{}

func (daySpanSoft) Name() string { return "day-span" }

func (c daySpanSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	var penalty float64
	var vs []Violation
	for i := range p.Invigilators {
		in := &p.Invigilators[i]

		type span struct{ first, last time.Time }
		byDay := make(map[int]*span)
		fold := func(day int, start, end time.Time) {
			if cur, ok := byDay[day]; ok {
				if start.Before(cur.first) {
					cur.first = start
				}
				if end.After(cur.last) {
					cur.last = end
				}
				return
			}
			byDay[day] = &span{first: start, last: end}
		}

		// presence from assigned invigilations
		for _, posIdx := range plan.Positions(in.ID) {
			pos := p.Positions[posIdx]
			fold(pos.Day, pos.Start, pos.End())
		}
		// presence from own exams, but only on days the person actually
		// invigilates – on a pure exam day the span is fixed and cannot be
		// reduced by the planner, so penalising it would be noise.
		for _, ex := range in.OwnExams {
			if _, invigilates := byDay[ex.Day]; invigilates {
				fold(ex.Day, ex.Start, ex.End)
			}
		}

		for day, sp := range byDay {
			hours := sp.last.Sub(sp.first).Hours()
			if hours > p.MaxSpanHours {
				over := hours - p.MaxSpanHours
				pen := p.Weights.DaySpan * over * over
				penalty += pen
				vs = append(vs, Violation{
					Constraint:    c.Name(),
					InvigilatorID: in.ID,
					Day:           day,
					Penalty:       pen,
					Message:       fmt.Sprintf("invigilator %d spans %.1fh on day %d (max %.0fh)", in.ID, hours, day, p.MaxSpanHours),
				})
			}
		}
	}
	return penalty, vs
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
