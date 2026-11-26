package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigplan"
)

// ValidateInvigilationConstraints checks the persisted invigilation plan
// (invigilations_self + invigilations_other) against the shared invigplan
// constraints – the exact same hard and soft rules the automatic generator
// uses. It runs in addition to the hand-written invigilator validations.
func (p *Plexams) ValidateInvigilationConstraints(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilation-constraints", "Aufsichtsplan gegen die Regeln der automatischen Einteilung prüfen")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	// Include every assignable invigilator so lookups of an already persisted
	// invigilator never miss.
	problem, err := p.buildInvigilationProblem(ctx, true)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("Die Daten für die Prüfung des Aufsichtsplans konnten nicht zusammengestellt werden: %v", err))
		return nil, err
	}
	name := p.teacherNames(ctx)

	// Build the plan from what is actually persisted, not from the fixed seeds.
	problem.Fixed = map[int]int{}
	plan := invigplan.NewPlan(problem)

	index := make(map[string]int, len(problem.Positions))
	for i, pos := range problem.Positions {
		index[positionKey(pos.Start, pos.IsReserve, pos.Room)] = i
	}

	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("Die Aufsichten konnten nicht gelesen werden: %v", err))
		return nil, err
	}

	for _, inv := range invigilations {
		isReserve := inv.RoomName == nil
		room := ""
		if inv.RoomName != nil {
			room = *inv.RoomName
		}
		if inv.Starttime == nil {
			continue
		}
		key := positionKey(*inv.Starttime, isReserve, room)
		idx, ok := index[key]
		if !ok {
			v.warnf(ref{Room: inv.RoomName, InvigilatorID: ptr(inv.InvigilatorID), Starttime: inv.Starttime}.hint(
				"Vermutlich wurden Prüfungen oder Räume nach dem Einteilen geändert. Unter „Aufsichten einteilen“ neu einteilen."),
				"%s ist am %s für %s eingeteilt, dort ist zu dieser Zeit aber keine Prüfung mehr geplant.",
				name(inv.InvigilatorID), slotLabelDE(*inv.Starttime), invigilationPlaceDE(inv.RoomName))
			continue
		}
		plan.Set(idx, inv.InvigilatorID)
	}

	// Check that every pre-planned invigilation is actually honored in the
	// persisted plan (a later manual change could have overridden it).
	prePlanned, err := p.PrePlannedInvigilations(ctx)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("Die vorgeplanten Aufsichten konnten nicht gelesen werden: %v", err))
		return nil, err
	}
	for _, pp := range prePlanned {
		room := "reserve"
		if pp.RoomName != nil {
			room = *pp.RoomName
		}
		if pp.Starttime == nil {
			continue
		}
		when := slotLabelDE(*pp.Starttime)
		place := invigilationPlaceDE(pp.RoomName)
		ppRef := ref{Room: pp.RoomName, InvigilatorID: ptr(pp.InvigilatorID), Starttime: pp.Starttime}
		idx, ok := index[positionKey(*pp.Starttime, pp.RoomName == nil, room)]
		if !ok {
			v.errorf(ppRef.hint("Die Vorplanung passt nicht mehr zur Raumplanung; solange sie besteht, bricht „Aufsichten einteilen“ mit einem Fehler ab. "+
				"Entferne die vorgeplante Aufsicht im „Aufsichtsplan“ und plane sie bei Bedarf neu."),
				"%s ist am %s für %s vorgeplant, dort ist zu dieser Zeit aber keine Prüfung geplant.",
				name(pp.InvigilatorID), when, place)
			continue
		}
		switch assigned := plan.Assign[idx]; {
		case assigned == invigplan.Unassigned:
			v.errorf(ppRef.hint("Unter „Aufsichten einteilen“ neu einteilen; vorgeplante Aufsichten werden dabei übernommen."),
				"%s ist am %s für %s vorgeplant, im Aufsichtsplan ist dort aber niemand eingeteilt.",
				name(pp.InvigilatorID), when, place)
		case assigned != pp.InvigilatorID:
			v.errorf(ppRef.hint("Unter „Aufsichten einteilen“ neu einteilen, dann gilt wieder die Vorplanung. "+
				"Ist die andere Person gewollt, im „Aufsichtsplan“ die Vorplanung entsprechend ändern."),
				"%s ist am %s für %s vorgeplant, eingeteilt ist dort aber %s.",
				name(pp.InvigilatorID), when, place, name(assigned))
		}
	}

	reg := invigplan.DefaultRegistry()
	hard := reg.HardViolations(problem, plan)
	_, costByConstraint, soft := reg.Cost(problem, plan)

	for _, viol := range hard {
		v.errorf(violationRef(viol).hint(invigRuleHint(viol.Constraint)), "%s", viol.Message)
	}
	for _, viol := range soft {
		v.warnf(violationRef(viol).hint(invigRuleHint(viol.Constraint)), "%s", viol.Message)
	}

	report := v.finish()

	// Stream the soft-constraint cost breakdown as informational text (internal
	// score, lower is better); it is not part of the structured findings.
	rules := make([]string, 0, len(costByConstraint))
	for rule := range costByConstraint {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	reporter.Println("Bewertung der Wünsche an die Einteilung (Strafpunkte, keine Minuten; kleiner ist besser):")
	for _, rule := range rules {
		if cost := costByConstraint[rule]; cost > 0 {
			reporter.Printf("    %-34s %8.0f\n", invigplan.ConstraintLabelDE(rule), cost)
		}
	}

	return report, nil
}

// violationRef links a rule violation to the person and the time it concerns.
func violationRef(viol invigplan.Violation) ref {
	var r ref
	if viol.InvigilatorID != 0 {
		r.InvigilatorID = ptr(viol.InvigilatorID)
	}
	if !viol.Start.IsZero() {
		r.Starttime = ptr(viol.Start)
	}
	return r
}

// invigRuleHint tells the planner what to do about a broken rule of the
// automatic invigilation planning (keyed by the rule's internal name).
func invigRuleHint(rule string) string {
	const reassign = "Gib die Aufsicht im „Aufsichtsplan“ an eine andere Person"
	switch rule {
	case "availability":
		return reassign + ", oder prüfe die Sperrtage: die aus dem ZPA stehen unter „Aufsichten mit Anforderungen“, zusätzliche unter „Aufsichts-Vorgaben“."
	case "time-window":
		return reassign + ", oder passe die Zeitfenster der Person unter „Aufsichts-Vorgaben“ an."
	case "own-exam", "one-per-slot":
		return reassign + "."
	case "time-gap":
		return "Gib eine der beiden Aufsichten im „Aufsichtsplan“ an eine andere Person."
	case "coverage":
		return "Trage im „Aufsichtsplan“ jemanden ein, oder teile unter „Aufsichten einteilen“ neu ein."
	case "minute-balance":
		return "Teile unter „Aufsichten einteilen“ neu ein, oder verschiebe im „Aufsichtsplan“ Aufsichten zu Personen mit offenen Minuten."
	default: // max-days, day-span, distribution-*: preferences, not errors
		return "Nur eine ungünstige Verteilung, kein Fehler. Wenn es stört, tausche im „Aufsichtsplan“ Aufsichten oder teile unter „Aufsichten einteilen“ neu ein."
	}
}

// positionKey is the lookup key matching a persisted invigilation to a problem
// position. It is keyed on the absolute start time (Unix seconds) instead of the
// former day/slot ordinals.
func positionKey(start time.Time, isReserve bool, room string) string {
	if isReserve {
		return fmt.Sprintf("%d/\x00reserve", start.Unix())
	}
	return fmt.Sprintf("%d/%s", start.Unix(), room)
}
