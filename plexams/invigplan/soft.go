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

		// Centering inside the band, scaled by the relative deviation: the band
		// edge of a person who has to do little is reached at a much smaller
		// absolute deviation than for a person with a large workload, so the same
		// absolute deviation costs proportionally more for the small-workload
		// person. Being under target is penalized harder than being over.
		eff := a
		if eff > p.ToleranceMin {
			eff = p.ToleranceMin // cap so the gentle term stays below the dominant tier
		}
		scale := in.TargetMinutes
		if scale < p.ToleranceMin {
			scale = p.ToleranceMin // floor avoids div-by-zero and over-penalizing tiny targets
		}
		rel := float64(eff) / float64(scale)
		if dev > 0 {
			rel *= p.Weights.OverTargetFactor // doing more than the target ("noch offen" < 0) is worse
		}
		within += rel

		if a > p.ToleranceMin {
			beyond += float64(a - p.ToleranceMin)
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: in.ID,
				Penalty:       p.Weights.BeyondTolerance * float64(a-p.ToleranceMin),
				Message:       minuteBalanceMessage(p.Name(in.ID), dev, doing, in.TargetMinutes, p.ToleranceMin),
			})
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
		vs = append(vs, Violation{
			Constraint: c.Name(),
			Start:      pos.Start,
			Penalty:    p.Weights.Coverage,
			Message:    fmt.Sprintf("Für %s am %s ist niemand eingeteilt.", placeDE(pos), timeDE(pos.Start)),
		})
	}
	return p.Weights.Coverage * float64(len(open)), vs
}

// maxDaysSoft: a person should be present on at most 3 days. Presence days are
// the union of their invigilation days and their own-exam days – but an excluded
// day does not count as presence (the person is treated as not there, even if an
// exam is recorded). Because the non-excluded own-exam days are unavoidable, the
// limit is raised to that count when it already exceeds 3, so only the *extra*
// days caused by invigilation are penalized.
type maxDaysSoft struct{}

func (maxDaysSoft) Name() string { return "max-days" }

const maxAttendanceDays = 3

func (c maxDaysSoft) Cost(p *Problem, plan *Plan) (float64, []Violation) {
	var penalty float64
	var vs []Violation
	for i := range p.Invigilators {
		in := &p.Invigilators[i]

		attendance := make(map[int]bool)
		for _, d := range plan.Days(in.ID) { // invigilation days are never excluded
			attendance[d] = true
		}
		examDays := 0
		for d := range in.OwnExamDays {
			if in.ExcludedDays[d] {
				continue // excluded day: treated as not present
			}
			attendance[d] = true
			examDays++
		}

		limit := max(maxAttendanceDays, examDays) // own exam days are unavoidable
		if len(attendance) > limit {
			excess := len(attendance) - limit
			pen := p.Weights.MaxDays * float64(excess*excess)
			penalty += pen
			vs = append(vs, Violation{
				Constraint:    c.Name(),
				InvigilatorID: in.ID,
				Penalty:       pen,
				Message: fmt.Sprintf("%s muss an %d Tagen an die Hochschule kommen (Aufsichten und eigene Prüfungen zusammen); angestrebt sind höchstens %d.",
					p.Name(in.ID), len(attendance), limit),
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
				Message: fmt.Sprintf("%s hat %d %s, im Durchschnitt sind es %s pro Person.",
					p.Name(id), n, kindPluralDE(c.kind), decimalDE(mean, 1)),
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
			fold(dateKey(pos.Start), pos.Start, pos.End())
		}
		// presence from own exams, but only on days the person actually
		// invigilates – on a pure exam day the span is fixed and cannot be
		// reduced by the planner, so penalising it would be noise.
		for _, ex := range in.OwnExams {
			if _, invigilates := byDay[ex.Date()]; invigilates {
				fold(ex.Date(), ex.Start, ex.End)
			}
		}

		for _, sp := range byDay {
			hours := sp.last.Sub(sp.first).Hours()
			if hours > p.MaxSpanHours {
				over := hours - p.MaxSpanHours
				pen := p.Weights.DaySpan * over * over
				penalty += pen
				vs = append(vs, Violation{
					Constraint:    c.Name(),
					InvigilatorID: in.ID,
					Start:         sp.first,
					Penalty:       pen,
					Message: fmt.Sprintf("%s ist am %s von %s bis %s Uhr eingebunden (Aufsichten und eigene Prüfungen), das sind %s Stunden; angestrebt sind höchstens %s Stunden.",
						p.Name(in.ID), dayDE(sp.first), sp.first.Format("15:04"), sp.last.Format("15:04"),
						decimalDE(hours, 1), decimalDE(p.MaxSpanHours, 0)),
				})
			}
		}
	}
	return penalty, vs
}

// minuteBalanceMessage says how far a person is off their target minutes.
func minuteBalanceMessage(name string, dev, doing, target, tolerance int) string {
	direction := "zu viel"
	if dev < 0 {
		direction = "zu wenig"
	}
	return fmt.Sprintf("%s hat %d Minuten Aufsicht bei einem Soll von %d Minuten, also %d Minuten %s (erlaubt sind ±%d Minuten).",
		name, doing, target, abs(dev), direction, tolerance)
}

// kindPluralDE names several positions of a kind, e.g. "Reserveaufsichten".
func kindPluralDE(k Kind) string {
	switch k {
	case KindReserve:
		return "Reserveaufsichten"
	case KindNTA:
		return "Aufsichten in NTA-Räumen"
	default:
		return "Raumaufsichten"
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
