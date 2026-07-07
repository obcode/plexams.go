package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigplan"
	"github.com/rs/zerolog/log"
)

// AssignInvigilations refreshes the self-invigilations and the todos, builds
// the planning problem, optimizes it and (unless dryRun) writes the result to
// invigilations_other, dropping the previous content. Self-invigilations stay
// in invigilations_self; pre-planned invigilations are included as fixed seeds.
// To fix an assignment across runs, move it to the pre-planning.
// ResetInvigilations drops the generated invigilations (invigilations_other) so
// that only the pre-planning (invigilations_pre_planned) remains; the
// self-invigilations are refreshed on the next generation. Blocked while the
// invigilation plan is published.
func (p *Plexams) ResetInvigilations(ctx context.Context) error {
	if err := p.generationAllowed(ctx, model.PlanningGateInvigilations); err != nil {
		return err
	}
	if err := p.dbClient.ResetGeneratedInvigilations(ctx); err != nil {
		return err
	}
	p.unmarkCondition(ctx, condInvigilationsAssigned)
	return nil
}

func (p *Plexams) AssignInvigilations(ctx context.Context, dryRun bool, opts invigplan.Options, reporter Reporter) (*model.InvigilationReport, error) {
	if err := p.generationAllowed(ctx, model.PlanningGateInvigilations); err != nil {
		return nil, err
	}
	reporter.Println("refreshing self-invigilations and todos ...")
	if err := p.PrepareSelfInvigilation(); err != nil {
		return nil, fmt.Errorf("cannot prepare self invigilations: %w", err)
	}
	if _, err := p.PrepareInvigilationTodos(ctx); err != nil {
		return nil, fmt.Errorf("cannot prepare invigilation todos: %w", err)
	}

	problem, err := p.buildInvigilationProblem(ctx, false)
	if err != nil {
		return nil, err
	}

	reporter.Printf("optimizing (up to %d iterations, seed %d) ...\n", opts.Iterations, opts.Seed)
	opts.ProgressEvery = max(1, opts.Iterations/200)
	opts.OnProgress = reporter.Progress

	best, result := invigplan.Optimize(problem, invigplan.DefaultRegistry(), opts)
	reporter.StopProgress(aurora.Sprintf(aurora.Green("optimization done")))

	report := printInvigilationReport(reporter, problem, best, result, opts)

	if dryRun {
		reporter.Println("dry run: nothing written")
		return report, nil
	}

	if !p.WritesAllowed() {
		return report, fmt.Errorf("writes are blocked while a validation is running")
	}

	if result.Unfilled > 0 {
		reporter.Warnf("writing plan with open positions: %d open", result.Unfilled)
		log.Warn().Int("open", result.Unfilled).Msg("writing plan with open positions")
	}

	toSave := make([]interface{}, 0, len(problem.Positions))
	for posIdx, invigID := range best.Assign {
		if invigID == invigplan.Unassigned {
			continue
		}
		pos := problem.Positions[posIdx]
		if pos.IsSelf {
			continue // already stored in invigilations_self
		}
		var roomName *string
		if !pos.IsReserve {
			name := pos.Room
			roomName = &name
		}
		// A fixed non-self position comes from the pre-planning.
		_, prePlanned := problem.Fixed[posIdx]
		start := pos.Start
		toSave = append(toSave, model.Invigilation{
			// Starttime is the source of truth; day/slot are derived on read.
			Starttime: &start,
			RoomName:  roomName,
			// Duration is the actual time block (longest invigilation in the
			// slot), not the credited minutes. For a reserve pos.Minutes is the
			// credited 60 min while pos.Block holds the slot's max duration; the
			// 60-min crediting happens in PrepareInvigilationTodos.
			Duration:           pos.Block,
			InvigilatorID:      invigID,
			IsReserve:          pos.IsReserve,
			IsSelfInvigilation: false,
			PrePlanned:         prePlanned,
		})
	}

	otherCtx := context.WithValue(ctx, db.CollectionName("collectionName"), "invigilations_other")
	if err := p.dbClient.DropAndSave(otherCtx, toSave); err != nil {
		return report, fmt.Errorf("cannot save generated invigilations: %w", err)
	}
	reporter.Printf("wrote %d invigilations to invigilations_other\n", len(toSave))

	reporter.Println("recalculating todos ...")
	if _, err := p.PrepareInvigilationTodos(ctx); err != nil {
		return report, fmt.Errorf("cannot recalculate todos: %w", err)
	}
	p.markCondition(ctx, condInvigilationsAssigned)
	reporter.Println("... done")
	return report, nil
}

// printInvigilationReport prints a readable, colored report of the optimizer
// outcome: the run status, whether the hard primary goals (balance, coverage)
// are met, the minute balance, the fairness of the reserve/NTA distribution and
// the breakdown of the soft-constraint cost.
// printInvigilationReport writes the readable, colored report to the reporter
// and returns the same outcome as structured data, so a GraphQL client can
// render it as a panel instead of parsing the text. The structured report is
// built even on a dry run.
func printInvigilationReport(reporter Reporter, problem *invigplan.Problem, plan *invigplan.Plan, result invigplan.Result, opts invigplan.Options) *model.InvigilationReport {
	stats := problem.Stats()
	nInvig := len(problem.Invigilators)
	m := problem.MinuteSummary(plan)

	reporter.Println()
	reporter.Println(aurora.Bold(aurora.Cyan(fmt.Sprintf("══ Invigilation plan  (seed %d, up to %d iterations) ══", opts.Seed, opts.Iterations))))

	// run status
	status := fmt.Sprintf("ran the full %d iterations", result.Iterations)
	if result.StoppedEarly {
		status = fmt.Sprintf("converged, stopped early after %d iterations", result.Iterations)
	}
	reporter.Printf("  %s %s\n", reportLabel("status"), status)

	// balance: the primary objective (everyone within ±tolerance of their target).
	if result.BalanceSatisfied {
		reporter.Printf("  %s %s\n", reportLabel("balance"),
			aurora.Green(fmt.Sprintf("✓ all %d invigilators within ±%d min of their target", nInvig, problem.ToleranceMin)))
	} else {
		reporter.Printf("  %s %s\n", reportLabel("balance"),
			aurora.Red(fmt.Sprintf("✗ %d over / %d under tolerance (worst +%d / -%d min)", m.Over, m.Under, m.MaxOver, m.MaxUnder)))
	}

	// coverage: every room and reserve has an invigilator.
	if result.Unfilled == 0 {
		reporter.Printf("  %s %s\n", reportLabel("coverage"),
			aurora.Green(fmt.Sprintf("✓ all %d positions filled", stats.Positions)))
	} else {
		reporter.Printf("  %s %s\n", reportLabel("coverage"),
			aurora.Red(fmt.Sprintf("✗ %d of %d positions still open", result.Unfilled, stats.Positions)))
	}

	// minute detail: how the assigned minutes sit around each person's target.
	reporter.Printf("  %s %s within ±%d min, %s over, %s under  %s\n", reportLabel("minutes"),
		aurora.Green(fmt.Sprintf("%d", m.WithinTolerance)), problem.ToleranceMin,
		colorCount(m.Over), colorCount(m.Under),
		aurora.Gray(12, "(deviation of assigned vs. target minutes per person)"))

	outliers := printDeviationOutliers(reporter, problem, plan)

	// fairness of the reserve / NTA distribution.
	reporter.Printf("  %s %s\n", reportLabel("fairness"),
		aurora.Gray(12, "(reading \"1:48\" = 48 invigilators do 1; lower max = fairer)"))
	fairness := make([]*model.FairnessDistribution, 0, 2)
	for _, kind := range []invigplan.Kind{invigplan.KindReserve, invigplan.KindNTA} {
		d := problem.DistributionOf(plan, kind)
		reporter.Printf("    %-9s %s  %s\n", kind.String()+":", d.String(),
			aurora.Gray(12, fmt.Sprintf("(%d total, max %d/person)", d.Total, d.Max)))
		fairness = append(fairness, fairnessModel(d))
	}

	// soft-constraint cost breakdown (internal score, lower is better).
	reporter.Printf("  %s %s\n", reportLabel("soft cost"),
		aurora.Sprintf(aurora.Gray(12, "total %.0f  (weighted penalty score, not minutes; lower is better)"), result.Cost))
	breakdown := result.SortedCosts()
	costItems := make([]*model.SoftCostItem, 0, len(breakdown))
	for _, b := range breakdown {
		reporter.Printf("    %-22s %8.0f\n", b.Name, b.Cost)
		costItems = append(costItems, &model.SoftCostItem{Name: b.Name, Cost: b.Cost})
	}

	return &model.InvigilationReport{
		Seed:          int(opts.Seed),
		Iterations:    opts.Iterations,
		IterationsRun: result.Iterations,
		StoppedEarly:  result.StoppedEarly,
		Balance: &model.BalanceReport{
			Satisfied:       result.BalanceSatisfied,
			Invigilators:    nInvig,
			ToleranceMin:    problem.ToleranceMin,
			WithinTolerance: m.WithinTolerance,
			Over:            m.Over,
			Under:           m.Under,
			MaxOver:         m.MaxOver,
			MaxUnder:        m.MaxUnder,
		},
		Coverage: &model.CoverageReport{
			Positions: stats.Positions,
			Unfilled:  result.Unfilled,
		},
		Minutes: &model.MinutesReport{
			WithinTolerance: m.WithinTolerance,
			Over:            m.Over,
			Under:           m.Under,
			ToleranceMin:    problem.ToleranceMin,
		},
		Outliers: outliers,
		Fairness: fairness,
		SoftCost: &model.SoftCostReport{
			Total:     result.Cost,
			Breakdown: costItems,
		},
	}
}

// fairnessModel converts an invigplan distribution into the GraphQL model,
// keeping the buckets sorted by count for a stable display.
func fairnessModel(d invigplan.Distribution) *model.FairnessDistribution {
	counts := d.SortedCounts()
	buckets := make([]*model.DistributionBucket, 0, len(counts))
	for _, n := range counts {
		buckets = append(buckets, &model.DistributionBucket{Count: n, Invigilators: d.ByCount[n]})
	}
	return &model.FairnessDistribution{
		Kind:    d.Kind.String(),
		Total:   d.Total,
		Max:     d.Max,
		Buckets: buckets,
	}
}

// printDeviationOutliers lists the invigilators whose assigned minutes are
// furthest from their target *relative to their workload*, so low-workload
// people that are still far off (especially under target) are easy to spot.
func printDeviationOutliers(reporter Reporter, problem *invigplan.Problem, plan *invigplan.Plan) []*model.InvigilatorOutlier {
	devs := problem.DeviationOutliers(plan, 5)
	if len(devs) == 0 {
		return nil
	}

	reporter.Printf("  %s %s\n", reportLabel("outliers"),
		aurora.Gray(12, "(noch offen = target − done, relative to workload; negative = did too much)"))
	outliers := make([]*model.InvigilatorOutlier, 0, len(devs))
	for _, d := range devs {
		line := fmt.Sprintf("invig %d: %d/%d min, noch offen %+d (%+.0f%%)", d.InvigilatorID, d.Doing, d.Target, d.Open, d.Percent)
		if d.Open < 0 { // did too much – the side we now penalize harder
			reporter.Printf("    %s\n", aurora.Yellow(line))
		} else {
			reporter.Printf("    %s\n", aurora.Gray(16, line))
		}
		outliers = append(outliers, &model.InvigilatorOutlier{
			InvigilatorID: d.InvigilatorID,
			Doing:         d.Doing,
			Target:        d.Target,
			Open:          d.Open,
			Percent:       d.Percent,
		})
	}
	return outliers
}

// reportLabel returns a fixed-width, bold label so the colored values line up.
func reportLabel(s string) string {
	return aurora.Bold(fmt.Sprintf("%-10s", s+":")).String()
}

// colorCount shows a count in green when it is zero, in yellow otherwise.
func colorCount(n int) aurora.Value {
	if n == 0 {
		return aurora.Green(fmt.Sprintf("%d", n))
	}
	return aurora.Yellow(fmt.Sprintf("%d", n))
}

// OptimizerOptionsFromConfig builds the optimizer options from viper, applying
// the given seed and iteration overrides (0 = keep config/default).
func (p *Plexams) OptimizerOptionsFromConfig(ctx context.Context, seed int64, iterations int) invigplan.Options {
	opts := invigplan.DefaultOptions()
	if cfg, err := p.GenerationConfig(ctx); err != nil {
		log.Error().Err(err).Msg("cannot get generation config, using defaults")
	} else {
		opts.Iterations = cfg.Iterations
		opts.StartTemp = cfg.StartTemp
		opts.EndTemp = cfg.EndTemp
	}
	if iterations > 0 {
		opts.Iterations = iterations
	}
	if seed != 0 {
		opts.Seed = seed
	}
	return opts
}

// ShowInvigilationProblem builds the invigilation snapshot from the DB and
// prints a summary. It is read-only and meant to sanity-check the inputs before
// the optimizer (Phase 3) is run.
func (p *Plexams) ShowInvigilationProblem(ctx context.Context) error {
	problem, err := p.buildInvigilationProblem(ctx, false)
	if err != nil {
		return err
	}
	s := problem.Stats()

	fmt.Printf("Invigilation problem\n")
	fmt.Printf("  positions:          %4d  (%d rooms, %d NTA rooms, %d reserves)\n",
		s.Positions, s.Rooms, s.NTARooms, s.Reserves)
	fmt.Printf("  fixed:              %4d  (%d self-invigilations + pre-planned)\n",
		s.FixedPositions, s.SelfPositions)
	fmt.Printf("  open to optimize:   %4d\n", s.Positions-s.FixedPositions)
	fmt.Printf("  invigilators:       %4d\n", s.Invigilators)
	fmt.Printf("  minutes to cover:   %4d  (target sum %d, tolerance ±%d)\n",
		s.SumPositionMinutes, s.SumTargetMinutes, problem.ToleranceMin)
	return nil
}

// buildInvigilationProblem assembles the static snapshot the invigilation
// optimizer works on. It reads everything from the DB once:
//   - positions to fill: every planned room per slot (one invigilator each) plus
//     one reserve per slot that has exams,
//   - invigilators with their target minutes and availability (from the cached
//     todos),
//   - the fixed assignments: self-invigilations and pre-planned invigilations,
//   - the own-exam slots that block a person (NTA exams also block the following
//     slot when they run into it).
//
// The returned Problem.Positions double as the write-back metadata: every
// assigned position carries its day/slot/room so the result can be stored in
// invigilations_other.
//
// With includeExcluded the over-contributed and free-semester invigilators are
// kept in the pool as well (target 0). The optimizer must exclude them, but the
// validation needs every assignable person so that lookups of an already
// persisted invigilator never miss.
func (p *Plexams) buildInvigilationProblem(ctx context.Context, includeExcluded bool) (*invigplan.Problem, error) {
	todos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		return nil, err
	}

	selfInvigilations, err := p.dbClient.GetSelfInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get self invigilations")
		return nil, err
	}
	// Day/slot ordinals are derived purely from the sorted config start times
	// (time-based, never persisted): dayIndexOf gives a stable 1-based calendar-day
	// index (matching semesterConfig.Days order), slotIndexOf a 1-based position within
	// the day, and nextStartOf the following start time on the same day. They replace
	// the former model.Slot.DayNumber/SlotNumber.
	starts := make([]time.Time, 0, len(p.semesterConfig.Slots))
	for _, slot := range p.semesterConfig.Slots {
		starts = append(starts, slot.Starttime)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })

	dayIndexByDate := make(map[string]int)
	slotsByDay := make(map[string][]time.Time)
	for _, t := range starts {
		key := t.Format("2006-01-02")
		if _, ok := dayIndexByDate[key]; !ok {
			dayIndexByDate[key] = len(dayIndexByDate) + 1
		}
		slotsByDay[key] = append(slotsByDay[key], t)
	}
	dayIndexOf := func(t time.Time) int { return dayIndexByDate[t.Format("2006-01-02")] }
	slotIndexOf := func(t time.Time) int {
		for i, s := range slotsByDay[t.Format("2006-01-02")] {
			if s.Equal(t) {
				return i + 1
			}
		}
		return 0
	}
	nextStartOf := func(t time.Time) (time.Time, bool) {
		day := slotsByDay[t.Format("2006-01-02")]
		for i, s := range day {
			if s.Equal(t) && i+1 < len(day) {
				return day[i+1], true
			}
		}
		return time.Time{}, false
	}

	// posKey identifies a room position by its slot start time (Unix seconds) and room.
	type posKey struct {
		start int64
		room  string
	}

	selfByPosition := make(map[posKey]int) // (start, room) -> examerID
	for _, si := range selfInvigilations {
		if si.RoomName == nil || si.Starttime == nil {
			continue
		}
		selfByPosition[posKey{si.Starttime.Unix(), *si.RoomName}] = si.InvigilatorID
	}

	prePlanned, err := p.PrePlannedInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre planned invigilations")
		return nil, err
	}

	ownExamSlots := make(map[int]map[[2]int]bool)
	ownExamDays := make(map[int]map[int]bool)
	ownExamTimes := make(map[int][]invigplan.TimeSpan)
	addOwnExam := func(examerID, day, slot int) {
		if ownExamSlots[examerID] == nil {
			ownExamSlots[examerID] = make(map[[2]int]bool)
			ownExamDays[examerID] = make(map[int]bool)
		}
		ownExamSlots[examerID][[2]int{day, slot}] = true
		ownExamDays[examerID][day] = true
	}

	positions := make([]invigplan.Position, 0)
	fixed := make(map[int]int)

	// posIndexByRoomAt and reserveAt let pre-planned invigilations find "their"
	// position again after the sweep, keyed on the slot's absolute start time.
	posIndexByRoomAt := make(map[posKey]int)
	reserveAt := make(map[int64]int)

	for _, slot := range p.semesterConfig.Slots {
		start := slot.Starttime
		day, sn := dayIndexOf(start), slotIndexOf(start)
		rooms, err := p.PlannedRoomsInSlot(ctx, start)
		if err != nil {
			log.Error().Err(err).Time("starttime", start).Msg("cannot get rooms in slot")
			return nil, err
		}
		if len(rooms) == 0 {
			continue
		}

		// own-exam slots from the exams in this slot (with NTA overrun).
		exams, err := p.ExamsAt(ctx, start)
		if err != nil {
			log.Error().Err(err).Time("starttime", start).Msg("cannot get exams in slot")
			return nil, err
		}
		nextStart, hasNext := nextStartOf(start)
		for _, exam := range exams {
			if exam.ZpaExam == nil || exam.PlanEntry == nil {
				continue
			}
			examerID := exam.ZpaExam.MainExamerID
			addOwnExam(examerID, day, sn)

			maxDur := 0
			for _, room := range exam.PlannedRooms {
				if room.Duration > maxDur {
					maxDur = room.Duration
				}
			}
			if maxDur > 0 {
				examEnd := start.Add(time.Duration(maxDur) * time.Minute)
				ownExamTimes[examerID] = append(ownExamTimes[examerID],
					invigplan.TimeSpan{Day: day, Start: start, End: examEnd})
				// NTA exams running into the following slot block it too.
				if hasNext && examEnd.After(nextStart) {
					addOwnExam(examerID, dayIndexOf(nextStart), slotIndexOf(nextStart))
				}
			}
		}

		// group planned rooms by name -> max duration, NTA flag.
		type roomInfo struct {
			maxDuration int
			isNTA       bool
		}
		roomMap := make(map[string]*roomInfo)
		slotMaxDuration := 0
		for _, room := range rooms {
			if room.Duration > slotMaxDuration {
				slotMaxDuration = room.Duration
			}
			info, ok := roomMap[room.RoomName]
			if !ok {
				info = &roomInfo{}
				roomMap[room.RoomName] = info
			}
			if room.Duration > info.maxDuration {
				info.maxDuration = room.Duration
			}
			if room.NtaMtknr != nil {
				info.isNTA = true
			}
		}

		roomNames := make([]string, 0, len(roomMap))
		for name := range roomMap {
			roomNames = append(roomNames, name)
		}
		sort.Strings(roomNames)

		for _, name := range roomNames {
			info := roomMap[name]
			pos := invigplan.Position{
				Day:     day,
				Slot:    sn,
				Room:    name,
				IsNTA:   info.isNTA,
				Minutes: info.maxDuration,
				Block:   info.maxDuration,
				Start:   start,
			}
			if examerID, ok := selfByPosition[posKey{start.Unix(), name}]; ok {
				pos.IsSelf = true
				pos.Minutes = 0
				fixed[len(positions)] = examerID
			}
			posIndexByRoomAt[posKey{start.Unix(), name}] = len(positions)
			positions = append(positions, pos)
		}

		// one reserve per slot with exams.
		reserveAt[start.Unix()] = len(positions)
		positions = append(positions, invigplan.Position{
			Day:       day,
			Slot:      sn,
			IsReserve: true,
			Minutes:   60, // matches SumReserve in PrepareInvigilationTodos
			Block:     slotMaxDuration,
			Start:     start,
		})
	}

	// apply pre-planned invigilations as fixed assignments.
	for _, pp := range prePlanned {
		var posIdx int
		ok := false
		if pp.Starttime != nil {
			if pp.RoomName == nil {
				posIdx, ok = reserveAt[pp.Starttime.Unix()]
			} else {
				posIdx, ok = posIndexByRoomAt[posKey{pp.Starttime.Unix(), *pp.RoomName}]
			}
		}
		if !ok {
			room := "reserve"
			if pp.RoomName != nil {
				room = *pp.RoomName
			}
			when := "unknown time"
			if pp.Starttime != nil {
				when = pp.Starttime.Format("02.01. 15:04")
			}
			log.Error().Int("invigilator", pp.InvigilatorID).Str("starttime", when).Str("room", room).
				Msg("pre-planned invigilation has no matching position (room/slot not planned)")
			return nil, fmt.Errorf("pre-planned invigilation for invigilator %d has no matching position: %s at %s is not planned",
				pp.InvigilatorID, room, when)
		}
		fixed[posIdx] = pp.InvigilatorID
	}

	invigilators := make([]invigplan.Invigilator, 0, len(todos.Invigilators))
	excluded := 0
	for _, inv := range todos.Invigilators {
		if inv.Teacher == nil {
			continue
		}
		// Take invigilators completely out of the pool who should do no
		// invigilation at all, so the balance criterion stays satisfiable and they
		// are never assigned:
		//   - free semester / not working (Factor <= 0),
		//   - already contributed more than their fair share (Todos.Enough).
		// Invigilators exactly at their fair share keep TargetMinutes 0 but stay in
		// the pool: the ±tolerance lets them absorb a little if it helps the others.
		overContributed := inv.Todos != nil && inv.Todos.Enough
		freeSemester := inv.Requirements != nil && inv.Requirements.Factor <= 0
		if overContributed || freeSemester {
			excluded++
			if !includeExcluded {
				continue
			}
		}
		id := inv.Teacher.ID
		gi := invigplan.Invigilator{
			ID:            id,
			ExcludedDays:  make(map[int]bool),
			ExcludedSlots: make(map[[2]int]bool),
			OwnExamSlots:  ownExamSlots[id],
			OwnExamDays:   ownExamDays[id],
			OwnExams:      ownExamTimes[id],
		}
		if gi.OwnExamSlots == nil {
			gi.OwnExamSlots = make(map[[2]int]bool)
		}
		if gi.OwnExamDays == nil {
			gi.OwnExamDays = make(map[int]bool)
		}
		if inv.Todos != nil {
			gi.TargetMinutes = inv.Todos.TotalMinutes
		}
		if inv.Requirements != nil {
			for _, day := range inv.Requirements.ExcludedDays {
				gi.ExcludedDays[day] = true
			}
			for _, w := range inv.Requirements.TimeWindows {
				if w == nil {
					continue
				}
				dtw := invigplan.DayTimeWindow{Date: w.Date}
				if w.From != nil {
					dtw.From = *w.From
				}
				if w.Until != nil {
					dtw.Until = *w.Until
				}
				gi.TimeWindows = append(gi.TimeWindows, dtw)
			}
		}
		invigilators = append(invigilators, gi)
	}

	cfg, err := p.GenerationConfig(ctx)
	if err != nil {
		return nil, err
	}
	problem := &invigplan.Problem{
		Positions:    positions,
		Invigilators: invigilators,
		Fixed:        fixed,
		TimelagMin:   p.generationTimelagMin(ctx),
		ToleranceMin: cfg.ToleranceMin,
		MaxSpanHours: cfg.MaxSpanHours,
		Weights:      optimizerWeights(cfg),
	}
	problem.Prepare()

	log.Debug().
		Int("positions", len(positions)).
		Int("fixed", len(fixed)).
		Int("invigilators", len(invigilators)).
		Int("excluded", excluded).
		Int("tolerance", problem.ToleranceMin).
		Msg("built invigilation problem")

	return problem, nil
}
