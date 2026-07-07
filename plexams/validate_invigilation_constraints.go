package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigplan"
)

// ValidateInvigilationConstraints checks the persisted invigilation plan
// (invigilations_self + invigilations_other) against the shared invigplan
// constraints – the exact same hard and soft rules the automatic generator
// uses. It runs in addition to the hand-written invigilator validations.
func (p *Plexams) ValidateInvigilationConstraints(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(p.TimeForSlot, reporter, "invigilation-constraints", "validating invigilation constraints (shared rules)")

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
		reporter.StopProgressFail(fmt.Sprintf("cannot build problem: %v", err))
		return nil, err
	}

	// Build the plan from what is actually persisted, not from the fixed seeds.
	problem.Fixed = map[int]int{}
	plan := invigplan.NewPlan(problem)

	index := make(map[string]int, len(problem.Positions))
	for i, pos := range problem.Positions {
		index[positionKey(pos.Day, pos.Slot, pos.IsReserve, pos.Room)] = i
	}

	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("cannot get invigilations: %v", err))
		return nil, err
	}

	for _, inv := range invigilations {
		isReserve := inv.RoomName == nil
		room := ""
		if inv.RoomName != nil {
			room = *inv.RoomName
		}
		key := positionKey(inv.Slot.DayNumber, inv.Slot.SlotNumber, isReserve, room)
		idx, ok := index[key]
		if !ok {
			where := room
			if isReserve {
				where = "reserve"
			}
			v.warnf(ref{Room: inv.RoomName, Day: ptr(inv.Slot.DayNumber), Slot: ptr(inv.Slot.SlotNumber), InvigilatorID: ptr(inv.InvigilatorID)},
				"invigilation for %s in slot (%d,%d) has no matching position (room/slot not planned)",
				where, inv.Slot.DayNumber, inv.Slot.SlotNumber)
			continue
		}
		plan.Set(idx, inv.InvigilatorID)
	}

	// Check that every pre-planned invigilation is actually honored in the
	// persisted plan (a later manual change could have overridden it).
	prePlanned, err := p.PrePlannedInvigilations(ctx)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("cannot get pre-planned invigilations: %v", err))
		return nil, err
	}
	for _, pp := range prePlanned {
		room := "reserve"
		if pp.RoomName != nil {
			room = *pp.RoomName
		}
		idx, ok := index[positionKey(pp.Day, pp.Slot, pp.RoomName == nil, room)]
		if !ok {
			v.errorf(ref{Room: pp.RoomName, Day: ptr(pp.Day), Slot: ptr(pp.Slot), InvigilatorID: ptr(pp.InvigilatorID)},
				"pre-planned %s in slot (%d,%d) has no matching position (room/slot not planned)",
				room, pp.Day, pp.Slot)
			continue
		}
		switch assigned := plan.Assign[idx]; {
		case assigned == invigplan.Unassigned:
			v.errorf(ref{Room: pp.RoomName, Day: ptr(pp.Day), Slot: ptr(pp.Slot), InvigilatorID: ptr(pp.InvigilatorID)},
				"pre-planned invigilator %d for %s in slot (%d,%d) is missing in the plan",
				pp.InvigilatorID, room, pp.Day, pp.Slot)
		case assigned != pp.InvigilatorID:
			v.errorf(ref{Room: pp.RoomName, Day: ptr(pp.Day), Slot: ptr(pp.Slot), InvigilatorID: ptr(pp.InvigilatorID)},
				"pre-planned invigilator %d for %s in slot (%d,%d) was overridden by %d",
				pp.InvigilatorID, room, pp.Day, pp.Slot, assigned)
		}
	}

	reg := invigplan.DefaultRegistry()
	hard := reg.HardViolations(problem, plan)
	_, costByConstraint, soft := reg.Cost(problem, plan)

	for _, viol := range hard {
		v.errorf(ref{}, "[%s] %s", viol.Constraint, viol.Message)
	}
	for _, viol := range soft {
		v.warnf(ref{}, "[%s] %s", viol.Constraint, viol.Message)
	}

	report := v.finish()

	// Stream the soft-constraint cost breakdown as informational text (internal
	// score, lower is better); it is not part of the structured findings.
	names := make([]string, 0, len(costByConstraint))
	for name := range costByConstraint {
		names = append(names, name)
	}
	sort.Strings(names)
	reporter.Println("soft-constraint cost (weighted penalty, not minutes):")
	for _, name := range names {
		if cost := costByConstraint[name]; cost > 0 {
			reporter.Printf("    %-22s %8.0f\n", name, cost)
		}
	}

	return report, nil
}

// positionKey is the lookup key matching a persisted invigilation to a problem
// position.
func positionKey(day, slot int, isReserve bool, room string) string {
	if isReserve {
		return fmt.Sprintf("%d/%d/\x00reserve", day, slot)
	}
	return fmt.Sprintf("%d/%d/%s", day, slot, room)
}
