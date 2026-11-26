package invigplan

import (
	"fmt"
	"strings"
	"time"
)

// The messages of the violations are read by the planner (validation of the
// invigilation plan), so they are German and name people, rooms and times the
// way the planner knows them.

// weekdaysDE maps time.Weekday (Sunday=0) to the German two-letter abbreviation.
var weekdaysDE = [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

// timeDE renders a start time as e.g. "Mo 25.01. 10:30".
func timeDE(t time.Time) string {
	return weekdaysDE[int(t.Weekday())] + " " + t.Format("02.01. 15:04")
}

// dayDE renders a date as e.g. "Mo 25.01.".
func dayDE(t time.Time) string {
	return weekdaysDE[int(t.Weekday())] + " " + t.Format("02.01.")
}

// decimalDE formats a number with a German decimal comma.
func decimalDE(f float64, prec int) string {
	return strings.Replace(fmt.Sprintf("%.*f", prec, f), ".", ",", 1)
}

// Name is the invigilator's name for a message; the id only as a fallback for a
// person the problem does not know by name.
func (p *Problem) Name(id int) string {
	if in := p.Invigilator(id); in != nil && in.Name != "" {
		return in.Name
	}
	return fmt.Sprintf("Unbekannte Aufsichtsperson (ZPA-Nr. %d)", id)
}

// placeDE names what a position is about: "Raum R1.006" or "die Reserveaufsicht".
func placeDE(pos Position) string {
	if pos.IsReserve || pos.Room == "" {
		return "die Reserveaufsicht"
	}
	return "Raum " + pos.Room
}

// dutyDE names the invigilation itself: "die Aufsicht in Raum R1.006" or "die Reserveaufsicht".
func dutyDE(pos Position) string {
	if pos.IsReserve || pos.Room == "" {
		return "die Reserveaufsicht"
	}
	return "die Aufsicht in Raum " + pos.Room
}

// ConstraintLabelDE is the planner's name of a rule, for reports. Unknown names
// are returned unchanged.
func ConstraintLabelDE(name string) string {
	switch name {
	case "availability":
		return "Sperrtage"
	case "time-window":
		return "Zeitfenster"
	case "own-exam":
		return "Eigene Prüfung"
	case "one-per-slot":
		return "Mehrere Aufsichten gleichzeitig"
	case "time-gap":
		return "Pause zwischen Aufsichten"
	case "minute-balance":
		return "Ausgleich der Minuten"
	case "coverage":
		return "Unbesetzte Aufsichten"
	case "max-days":
		return "Anwesenheitstage"
	case "prefer-own-exam-days":
		return "Aufsicht an eigenen Prüfungstagen"
	case "distribution-reserve":
		return "Verteilung der Reserveaufsichten"
	case "distribution-nta":
		return "Verteilung der NTA-Räume"
	case "distribution-room":
		return "Verteilung der Raumaufsichten"
	case "day-span":
		return "Länge des Tages"
	}
	return name
}

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
				Message: fmt.Sprintf("%s ist am %s für %s eingeteilt, hat diesen Tag bzw. diesen Termin aber gesperrt.",
					p.Name(invigID), timeDE(pos.Start), placeDE(pos)),
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
		vs = append(vs, Violation{
			Constraint:    c.Name(),
			InvigilatorID: invigID,
			Start:         pos.Start,
			Message: fmt.Sprintf("%s ist am %s für %s eingeteilt (%s–%s Uhr). Das liegt außerhalb der Zeiten, die für diese Person an diesem Tag eingetragen sind.",
				p.Name(invigID), dayDE(pos.Start), placeDE(pos),
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
				Message: fmt.Sprintf("%s ist am %s für %s eingeteilt, hat zu dieser Zeit aber eine eigene Prüfung.",
					p.Name(invigID), timeDE(pos.Start), placeDE(pos)),
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
				Message: fmt.Sprintf("%s ist am %s mehrfach gleichzeitig eingeteilt (unter anderem für %s).",
					p.Name(invigID), timeDE(pos.Start), placeDE(pos)),
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
					if b.Start.Before(a.Start) {
						a, b = b, a
					}
					vs = append(vs, Violation{
						Constraint:    c.Name(),
						InvigilatorID: invigID,
						Start:         a.Start,
						Message: fmt.Sprintf("%s hat am %s zwei Aufsichten zu dicht hintereinander: %s endet um %s, %s beginnt um %s. Nötig sind mindestens %d Minuten Pause.",
							p.Name(invigID), dayDE(a.Start), dutyDE(a), a.End().Format("15:04"),
							dutyDE(b), b.Start.Format("15:04"), p.TimelagMin),
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
