package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/plexams/invigplan"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

// ValidateInvigilationConstraints checks the persisted invigilation plan
// (invigilations_self + invigilations_other) against the shared invigplan
// constraints – the exact same hard and soft rules the automatic generator
// uses. It runs in addition to the hand-written invigilator validations.
func (p *Plexams) ValidateInvigilationConstraints() error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating invigilation constraints (shared rules)")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}
	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	_ = spinner.Start()

	ctx := context.Background()

	// Include every assignable invigilator so lookups of an already persisted
	// invigilator never miss.
	problem, err := p.buildInvigilationProblem(ctx, true)
	if err != nil {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("cannot build problem: %v"), err))
		_ = spinner.StopFail()
		return err
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
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("cannot get invigilations: %v"), err))
		_ = spinner.StopFail()
		return err
	}

	orphans := make([]string, 0)
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
			orphans = append(orphans, aurora.Sprintf(aurora.Yellow("invigilation for %s in slot (%d,%d) has no matching position (room/slot not planned)"),
				aurora.Magenta(where), aurora.Cyan(inv.Slot.DayNumber), aurora.Cyan(inv.Slot.SlotNumber)))
			continue
		}
		plan.Set(idx, inv.InvigilatorID)
	}

	reg := invigplan.DefaultRegistry()
	hard := reg.HardViolations(problem, plan)
	_, costByConstraint, soft := reg.Cost(problem, plan)

	spinner.Stop() //nolint:errcheck

	// Report: hard violations first (must hold), then soft ones (should hold).
	if len(hard) == 0 {
		fmt.Println(aurora.Sprintf(aurora.Green("  ✓ no hard-constraint violations")))
	} else {
		fmt.Println(aurora.Sprintf(aurora.Red("  ✗ %d hard-constraint violation(s):"), len(hard)))
		for _, v := range hard {
			fmt.Printf("    %s %s\n", aurora.Red("✗"), aurora.Sprintf(aurora.Red("[%s] %s"), v.Constraint, v.Message))
		}
	}

	for _, msg := range orphans {
		fmt.Printf("    %s %s\n", aurora.Yellow("!"), msg)
	}

	if len(soft) == 0 {
		fmt.Println(aurora.Sprintf(aurora.Green("  ✓ no soft-constraint violations")))
	} else {
		fmt.Println(aurora.Sprintf(aurora.Yellow("  %d soft-constraint note(s):"), len(soft)))
		for _, v := range soft {
			fmt.Printf("    %s %s\n", aurora.Yellow("·"), aurora.Sprintf(aurora.Yellow("[%s] %s"), v.Constraint, v.Message))
		}
	}

	// Cost breakdown (internal score, lower is better).
	names := make([]string, 0, len(costByConstraint))
	for name := range costByConstraint {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println(aurora.Gray(12, "  soft-constraint cost (weighted penalty, not minutes):").String())
	for _, name := range names {
		if cost := costByConstraint[name]; cost > 0 {
			fmt.Printf("    %s\n", aurora.Gray(12, fmt.Sprintf("%-22s %8.0f", name, cost)))
		}
	}

	return nil
}

// positionKey is the lookup key matching a persisted invigilation to a problem
// position.
func positionKey(day, slot int, isReserve bool, room string) string {
	if isReserve {
		return fmt.Sprintf("%d/%d/\x00reserve", day, slot)
	}
	return fmt.Sprintf("%d/%d/%s", day, slot, room)
}
