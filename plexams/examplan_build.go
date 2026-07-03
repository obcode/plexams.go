package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/examplan"
	"github.com/obcode/plexams.go/plexams/optimize"
	"github.com/rs/zerolog/log"
)

// smallExamThreshold: exams with at most this many registrations are "small" and, for
// the same examer, preferably scheduled into the same slot.
const smallExamThreshold = 5

// defaultExamGapMinutes is the travel/break buffer a student needs between two
// consecutive exams. If an exam's duration (NTA-extended for the affected student) plus
// this buffer reaches into the next slot, that student may not sit in the next slot.
// Overridable via planer.examGapMinutes.
const defaultExamGapMinutes = 30

// ExamScheduleResult summarizes a Terminplan generation run.
type ExamScheduleResult struct {
	Units             int
	Fixed             int
	Placed            int
	Unplaced          int
	UnplacedAncodes   []int
	HardViolations    []string
	Cost              float64
	CostByConstraint  map[string]float64
	Iterations        int
	StoppedEarly      bool
	Written           bool
	Seed              int
	Diagnostics       examplan.Diagnostics
	Conflicts         []*model.ExamScheduleConflict
	ResolvedConflicts []*model.ExamScheduleConflict
}

// buildExamPlanProblem assembles the exam-schedule optimization problem from the
// current data: assembled exams to plan (movable), fixed obstacles (locked / external
// / not-planned-by-me), per-student conflict pairs, EXaHM slot capacities and the
// attract pairs (parallel sections / small same-examer exams).
func (p *Plexams) buildExamPlanProblem(ctx context.Context, applyRatings, roomPhase bool) (*examplan.Problem, error) {
	sc := p.semesterConfig
	if sc == nil {
		return nil, fmt.Errorf("no semester config loaded")
	}

	// --- slots ---
	slotIdx := make(map[[2]int]int, len(sc.Slots))
	slotKeys := make([][2]int, 0, len(sc.Slots))
	slots := make([]examplan.Slot, 0, len(sc.Slots))
	for _, s := range sc.Slots {
		key := [2]int{s.DayNumber, s.SlotNumber}
		slotIdx[key] = len(slots)
		slotKeys = append(slotKeys, key)
		slots = append(slots, examplan.Slot{
			SlotRef: examplan.SlotRef{Day: s.DayNumber, Slot: s.SlotNumber, Start: s.Starttime},
			// Seats (total room capacity per slot) is left 0 = unlimited for now; TODO.
		})
	}
	if booked, err := p.annyBookedBySlot(ctx, slotKeys); err == nil {
		for key, sb := range booked {
			if idx, ok := slotIdx[key]; ok && sb != nil {
				slots[idx].ExahmSeats = sb.exahmSeats
				slots[idx].SebSeats = sb.sebSeats
			}
		}
	} else {
		log.Warn().Err(err).Msg("cannot read anny bookings; EXaHM/SEB slots treated as none")
	}

	// --- assembled exams, plan entries, constraints ---
	assembled, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		return nil, err
	}
	planEntries, err := p.PlanEntries(ctx)
	if err != nil {
		return nil, err
	}
	peByAncode := make(map[int]*model.PlanEntry, len(planEntries))
	for _, pe := range planEntries {
		peByAncode[pe.Ancode] = pe
	}
	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		return nil, err
	}

	type exRec struct {
		e          *model.AssembledExam
		fixedSlot  int // -1 if movable
		allowed    []int
		foreign    bool
		exahm, seb bool
	}
	rec := make(map[int]*exRec, len(assembled))
	noRegsSkipped := make([]int, 0)
	for _, e := range assembled {
		c := constraints[e.Ancode]
		pe := peByAncode[e.Ancode]
		exahm := c != nil && c.RoomConstraints != nil && c.RoomConstraints.Exahm
		seb := c != nil && c.RoomConstraints != nil && c.RoomConstraints.Seb
		foreign := (c != nil && c.NotPlannedByMe) || (pe != nil && pe.ExternalTime != nil) || e.Ancode >= externalAncodeBase
		// no registrations → no exam: our own 0-registration exams are not planned
		// (dynamic — reincluded automatically once registrations appear). Foreign exams
		// keep 0 of our regs on purpose and stay as time obstacles.
		if e.StudentRegsCount == 0 && !foreign {
			noRegsSkipped = append(noRegsSkipped, e.Ancode)
			continue
		}
		// Phase A (roomPhase): only EXaHM/SEB are movable; phaseFixed is ignored (we
		// re-place them). Phase B: phaseFixed entries are fixed obstacles.
		fixed := foreign || (pe != nil && pe.Locked)
		if !roomPhase && pe != nil && pe.PhaseFixed {
			fixed = true
		}
		if fixed {
			if pe == nil {
				continue // fixed but no slot known → not schedulable, no obstacle
			}
			idx, ok := slotIdx[[2]int{pe.DayNumber, pe.SlotNumber}]
			if !ok {
				continue
			}
			rec[e.Ancode] = &exRec{e: e, fixedSlot: idx, foreign: foreign, exahm: exahm, seb: seb}
			continue
		}
		// movable
		if roomPhase && !exahm && !seb {
			continue // phase A schedules only EXaHM/SEB exams
		}
		allowedSlots, err := p.AllowedSlots(ctx, e.Ancode)
		if err != nil {
			return nil, fmt.Errorf("allowed slots for %d: %w", e.Ancode, err)
		}
		idxs := make([]int, 0, len(allowedSlots))
		for _, s := range allowedSlots {
			if idx, ok := slotIdx[[2]int{s.DayNumber, s.SlotNumber}]; ok {
				idxs = append(idxs, idx)
			}
		}
		rec[e.Ancode] = &exRec{e: e, fixedSlot: -1, allowed: idxs, exahm: exahm, seb: seb}
	}
	if len(noRegsSkipped) > 0 {
		sort.Ints(noRegsSkipped)
		log.Info().Ints("ancodes", noRegsSkipped).Int("count", len(noRegsSkipped)).
			Msg("exams without registrations are not planned")
	}

	ancodes := make([]int, 0, len(rec))
	for a := range rec {
		ancodes = append(ancodes, a)
	}
	sort.Ints(ancodes)

	// --- same-slot union-find among movable exams ---
	parent := make(map[int]int, len(ancodes))
	for _, a := range ancodes {
		parent[a] = a
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, a := range ancodes {
		if rec[a].fixedSlot >= 0 {
			continue
		}
		if c := constraints[a]; c != nil {
			for _, other := range c.SameSlot {
				if ro, ok := rec[other]; ok && ro.fixedSlot < 0 {
					parent[find(a)] = find(other)
				}
			}
		}
	}

	// --- units ---
	units := make([]examplan.Unit, 0, len(ancodes))
	unitOf := make(map[int]int, len(ancodes))
	unitRepeater := []bool{}
	unitSemester := []int{}

	// movable units grouped by same-slot root
	groups := make(map[int][]int)
	roots := make([]int, 0)
	for _, a := range ancodes {
		if rec[a].fixedSlot >= 0 {
			continue
		}
		r := find(a)
		if _, seen := groups[r]; !seen {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], a)
	}
	sort.Ints(roots)
	for _, r := range roots {
		members := groups[r]
		sort.Ints(members)
		idx := len(units)
		u := examplan.Unit{ID: members[0]}
		allowedSets := make([][]int, 0, len(members))
		repeater := false
		minSem := 0
		pinnedSlot := -1 // a fixed exam this group is sameSlot with: the movable group must go there
		for _, a := range members {
			e := rec[a].e
			u.Ancodes = append(u.Ancodes, a)
			u.Seats += e.StudentRegsCount
			if rec[a].exahm {
				u.Exahm = true
			}
			if rec[a].seb {
				u.Seb = true
			}
			if c := constraints[a]; c != nil {
				// sameSlot with a FIXED exam pins this group to that exam's slot (the
				// movable-only union-find above cannot merge across fixed exams).
				for _, other := range c.SameSlot {
					if ro, ok := rec[other]; ok && ro.fixedSlot >= 0 {
						if pinnedSlot >= 0 && pinnedSlot != ro.fixedSlot {
							log.Warn().Int("ancode", a).Msg("sameSlot with fixed exams in different slots — cannot satisfy both")
						}
						pinnedSlot = ro.fixedSlot
					}
				}
			}
			if e.ZpaExam.IsRepeaterExam {
				repeater = true
			}
			if s := minGroupSemester(e.ZpaExam.Groups); s > 0 && (minSem == 0 || s < minSem) {
				minSem = s
			}
			allowedSets = append(allowedSets, rec[a].allowed)
			unitOf[a] = idx
		}
		e0 := rec[members[0]].e
		u.Examer = e0.ZpaExam.MainExamerID
		u.Module = e0.ZpaExam.Module
		u.Program = firstProgram(e0)
		u.Location = locationOf(constraints[members[0]])
		if pinnedSlot >= 0 {
			u.Allowed = []int{pinnedSlot} // pinned to the fixed sameSlot partner's slot
		} else {
			u.Allowed = intersectAllowed(allowedSets)
		}
		// warm-start slot = this exam's current plan entry (if any)
		u.StartSlot = -1
		if pe := peByAncode[members[0]]; pe != nil {
			if idx, ok := slotIdx[[2]int{pe.DayNumber, pe.SlotNumber}]; ok {
				u.StartSlot = idx
			}
		}
		units = append(units, u)
		unitRepeater = append(unitRepeater, repeater)
		unitSemester = append(unitSemester, minSem)
	}

	// fixed units (obstacles)
	for _, a := range ancodes {
		r := rec[a]
		if r.fixedSlot < 0 {
			continue
		}
		idx := len(units)
		units = append(units, examplan.Unit{
			ID: a, Ancodes: []int{a}, Seats: r.e.StudentRegsCount, Exahm: r.exahm, Seb: r.seb,
			Examer: r.e.ZpaExam.MainExamerID, Module: r.e.ZpaExam.Module, Program: firstProgram(r.e),
			Fixed: true, FixedSlot: r.fixedSlot, Foreign: r.foreign, Location: locationOf(constraints[a]),
			StartSlot: -1,
		})
		unitOf[a] = idx
		unitRepeater = append(unitRepeater, r.e.ZpaExam.IsRepeaterExam)
		unitSemester = append(unitSemester, minGroupSemester(r.e.ZpaExam.Groups))
	}

	// --- conflict ratings & canShareSlot (keyed by unit pair) ---
	canShare := make(map[[2]int]bool)
	decisions := make(map[string]map[[2]int]model.ConflictDecision) // mtknr -> unit pair -> decision
	unitPair := func(a, b int) ([2]int, bool) {
		ua, ok1 := unitOf[a]
		ub, ok2 := unitOf[b]
		if !ok1 || !ok2 || ua == ub {
			return [2]int{}, false
		}
		if ua > ub {
			ua, ub = ub, ua
		}
		return [2]int{ua, ub}, true
	}
	// sameSlot exams must run at the same time, so a shared student may of course have
	// both in one slot — treat such pairs like canShareSlot (no same-slot veto/penalty
	// between them). Movable-movable sameSlot are one merged unit already (unitPair
	// returns false); this covers movable<->fixed sameSlot (distinct units).
	for _, a := range ancodes {
		if c := constraints[a]; c != nil {
			for _, other := range c.SameSlot {
				if up, ok := unitPair(a, other); ok {
					canShare[up] = true
				}
			}
		}
	}
	if applyRatings {
		if pairs, err := p.dbClient.CanShareSlotPairs(ctx); err == nil {
			for _, pr := range pairs {
				if up, ok := unitPair(pr[0], pr[1]); ok {
					canShare[up] = true
				}
			}
		}
		if decs, err := p.dbClient.StudentConflictDecisions(ctx); err == nil {
			for _, d := range decs {
				up, ok := unitPair(d.Ancode1, d.Ancode2)
				if !ok {
					continue
				}
				if decisions[d.Mtknr] == nil {
					decisions[d.Mtknr] = make(map[[2]int]model.ConflictDecision)
				}
				decisions[d.Mtknr][up] = d.Decision
			}
		}
	}

	// --- students / conflict pairs ---
	studentsRaw, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		return nil, err
	}

	// MUC.DAI restriction: any exam a MUC.DAI-program student (DE/GS/ID) is registered
	// for must be scheduled in a MUC.DAI slot — this includes normal ZPA exams (e.g.
	// 134/118) that a MUC.DAI student takes, not only the external MUC.DAI exams.
	mucDaiProg := make(map[string]bool)
	for _, prog := range p.mucdaiProgramNames(ctx) {
		mucDaiProg[prog] = true
	}
	mucDaiSlotIdx := make([]int, 0)
	for _, s := range sc.MucDaiSlots {
		if idx, ok := slotIdx[[2]int{s.DayNumber, s.SlotNumber}]; ok {
			mucDaiSlotIdx = append(mucDaiSlotIdx, idx)
		}
	}
	if len(mucDaiProg) > 0 && len(mucDaiSlotIdx) > 0 {
		mucDaiUnit := make(map[int]bool)
		for _, s := range studentsRaw {
			for _, rwp := range s.RegsWithProgram {
				if mucDaiProg[rwp.Program] {
					if u, ok := unitOf[rwp.Reg]; ok && !units[u].Fixed {
						mucDaiUnit[u] = true
					}
				}
			}
		}
		for u := range mucDaiUnit {
			inter := intersectSlots(units[u].Allowed, mucDaiSlotIdx)
			if len(inter) == 0 {
				inter = []int{-1} // no MUC.DAI slot fits its other constraints → unplaceable (reported)
			}
			units[u].Allowed = inter
		}
	}

	w := examplan.DefaultWeights()
	if roomPhase {
		// Phase A: main goal is to fill the booked T-building rooms with EXaHM/SEB;
		// even distribution over all slots is off (we want concentration in T-Bau).
		w.TbauFill = 10000
		w.SlotLoad = 0
	}
	students := make([]examplan.Student, 0, len(studentsRaw))
	for _, s := range studentsRaw {
		seen := make(map[int]bool)
		list := make([]int, 0, len(s.Regs))
		for _, ancode := range s.Regs {
			if ui, ok := unitOf[ancode]; ok && !seen[ui] {
				seen[ui] = true
				list = append(list, ui)
			}
		}
		if len(list) < 2 {
			continue
		}
		sort.Ints(list)
		studSem := semesterOf(s.Group)
		var pairs []examplan.Pair
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				a, b := list[i], list[j]
				if units[a].Foreign && units[b].Foreign {
					continue // both external: not ours to optimize, kept out of stats
				}
				up := [2]int{a, b}
				if canShare[up] {
					continue // declared shareable: drop hard + soft entirely
				}
				isRepeat := repeatForStudent(studSem, unitRepeater[a], unitSemester[a]) ||
					repeatForStudent(studSem, unitRepeater[b], unitSemester[b])
				weight := 1.0
				switch dec := decisions[s.Mtknr][up]; {
				case dec == model.ConflictDecisionAccept:
					weight = 0 // accepted: no proximity penalty (same-slot still hard)
				case dec == model.ConflictDecisionVeto:
					weight = 1.0 // vetoed: counts fully, overriding the repeat down-weight
				case isRepeat:
					weight = w.RepeatFactor // auto down-weight for (likely) repeats
				}
				pairs = append(pairs, examplan.Pair{A: a, B: b, Weight: weight, CrossLoc: units[a].Location != units[b].Location})
			}
		}
		if len(pairs) > 0 {
			students = append(students, examplan.Student{ID: s.Mtknr, Pairs: pairs})
		}
	}
	// deterministic order (the DB/query order is not guaranteed) so a re-run with the
	// same seed and data yields the exact same plan
	sort.Slice(students, func(i, j int) bool { return students[i].ID < students[j].ID })

	// --- attract pairs ---
	attractSet := make(map[[2]int]bool)
	addAttract := func(a, b int) {
		if a == b {
			return
		}
		if a > b {
			a, b = b, a
		}
		attractSet[[2]int{a, b}] = true
	}
	byModProg := make(map[string][]int)
	byExamer := make(map[int][]int)
	for i := range units {
		if units[i].Fixed {
			continue
		}
		byModProg[units[i].Module+"|"+units[i].Program] = append(byModProg[units[i].Module+"|"+units[i].Program], i)
		if units[i].Seats <= smallExamThreshold && units[i].Examer != 0 {
			byExamer[units[i].Examer] = append(byExamer[units[i].Examer], i)
		}
	}
	for _, list := range byModProg { // parallel sections: same module+program, different examer
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if units[list[i]].Examer != units[list[j]].Examer {
					addAttract(list[i], list[j])
				}
			}
		}
	}
	for _, list := range byExamer { // small exams of the same examer
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				addAttract(list[i], list[j])
			}
		}
	}
	attract := make([]examplan.AttractPair, 0, len(attractSet))
	for k := range attractSet {
		attract = append(attract, examplan.AttractPair{A: k[0], B: k[1], Weight: 1})
	}
	sort.Slice(attract, func(i, j int) bool {
		if attract[i].A != attract[j].A {
			return attract[i].A < attract[j].A
		}
		return attract[i].B < attract[j].B
	})

	// --- consecutive-exam gap (hard): a student needs a travel/break buffer between two
	// of their exams; an NTA time extension eats into it. If a student's occupied time
	// for exam A (its duration, extended for that student's NTA) plus the buffer reaches
	// into the next slot, they may not take another of their exams in the following slot.
	gapMin := sc.ExamGapMinutes
	if gapMin <= 0 {
		gapMin = defaultExamGapMinutes
	}
	blockMin := int(slotBlockDuration(sc.Starttimes).Minutes())
	unitBaseDur := make(map[int]int)       // unit -> exam duration
	ntaExt := make(map[int]map[string]int) // unit -> mtknr -> NTA-extended duration
	for _, e := range assembled {
		ui, ok := unitOf[e.Ancode]
		if !ok || e.ZpaExam == nil {
			continue
		}
		if e.ZpaExam.Duration > unitBaseDur[ui] {
			unitBaseDur[ui] = e.ZpaExam.Duration
		}
		for _, nta := range e.Ntas {
			ext := e.ZpaExam.Duration * (100 + nta.DeltaDurationPercent) / 100
			if ntaExt[ui] == nil {
				ntaExt[ui] = make(map[string]int)
			}
			if ext > ntaExt[ui][nta.Mtknr] {
				ntaExt[ui][nta.Mtknr] = ext
			}
		}
	}
	// overrunsFor: does exam `unit` leave the student too little time before the next
	// slot (duration + buffer reaches into it)?
	overrunsFor := func(unit int, mtknr string) bool {
		dur := unitBaseDur[unit]
		if m := ntaExt[unit]; m != nil {
			if ext, ok := m[mtknr]; ok && ext > dur {
				dur = ext
			}
		}
		return blockMin > 0 && dur+gapMin > blockMin
	}
	gapForbiddenSet := make(map[[2]int]bool)
	for _, s := range studentsRaw {
		uset := make(map[int]bool)
		for _, ancode := range s.Regs {
			if ui, ok := unitOf[ancode]; ok {
				uset[ui] = true
			}
		}
		if len(uset) < 2 {
			continue
		}
		for a := range uset {
			if !overrunsFor(a, s.Mtknr) {
				continue
			}
			for b := range uset { // a overruns → b must not sit in the slot right after a
				if a != b {
					gapForbiddenSet[[2]int{a, b}] = true
				}
			}
		}
	}
	gapForbidden := make([][2]int, 0, len(gapForbiddenSet))
	for k := range gapForbiddenSet {
		gapForbidden = append(gapForbidden, k)
	}
	sort.Slice(gapForbidden, func(i, j int) bool {
		if gapForbidden[i][0] != gapForbidden[j][0] {
			return gapForbidden[i][0] < gapForbidden[j][0]
		}
		return gapForbidden[i][1] < gapForbidden[j][1]
	})

	prob := examplan.NewProblem(slots, units, students, attract, w)
	prob.SetNTAOverruns(gapForbidden)
	return prob, nil
}

// GenerateExamSchedule builds and solves the exam schedule, streaming progress to the
// reporter. With dryRun it only reports (nothing written); otherwise it writes the
// non-fixed plan entries (locked / external / not-planned-by-me stay untouched) and
// removes stale entries of any exam that ended up unplaced. It refuses to write when
// there are hard violations.
func (p *Plexams) GenerateExamSchedule(ctx context.Context, dryRun bool, seed int64, iterations int, ignoreRatings, keepAssigned bool, reporter Reporter) (*ExamScheduleResult, error) {
	return p.runExamGeneration(ctx, false, dryRun, seed, iterations, ignoreRatings, keepAssigned, reporter, condExamScheduleGenerated)
}

// GenerateExamRoomsPhase runs phase A: it schedules only the EXaHM/SEB exams into the
// booked T-building slots (maximizing room usage), leaving everything else for phase B.
func (p *Plexams) GenerateExamRoomsPhase(ctx context.Context, dryRun bool, seed int64, iterations int, reporter Reporter) (*ExamScheduleResult, error) {
	return p.runExamGeneration(ctx, true, dryRun, seed, iterations, false, false, reporter, condExahmSebPlanned)
}

func (p *Plexams) runExamGeneration(ctx context.Context, roomPhase, dryRun bool, seed int64, iterations int, ignoreRatings, keepAssigned bool, reporter Reporter, doneCond string) (*ExamScheduleResult, error) {
	if ignoreRatings {
		reporter.Println("Konflikt-Bewertungen werden für diesen Lauf ignoriert")
	}
	if keepAssigned {
		reporter.Println("Warm-Start: bestehender Plan wird als Ausgangspunkt behalten")
	}
	if roomPhase {
		reporter.Step("EXaHM/SEB-Raumphase wird aufgebaut …")
	} else {
		reporter.Step("Terminplan-Problem wird aufgebaut …")
	}
	prob, err := p.buildExamPlanProblem(ctx, !ignoreRatings, roomPhase)
	if err != nil {
		reporter.StopProgressFail("Aufbau fehlgeschlagen: " + err.Error())
		return nil, err
	}
	movable := 0
	for i := range prob.Units {
		if !prob.Units[i].Fixed {
			movable++
		}
	}
	what := "Prüfungen zu planen"
	if roomPhase {
		what = "EXaHM/SEB-Prüfungen zu platzieren"
	}
	reporter.Println(fmt.Sprintf("%d %s, %d fest, %d Slots, %d Studierende mit Konflikten",
		movable, what, len(prob.Units)-movable, len(prob.Slots), len(prob.Students)))
	if n := prob.NumNTAOverruns(); n > 0 {
		reporter.Println(fmt.Sprintf("%d Zwischenzeit-Sperren berücksichtigt (Folgeslot, inkl. NTA)", n))
	}

	opts := optimize.DefaultOptions()
	opts.Seed = seed
	opts.StrictImprove = keepAssigned // warm start: only strictly-improving moves (low churn)
	if iterations > 0 {
		opts.Iterations = iterations
	}
	opts.ProgressEvery = maxInt(1, opts.Iterations/200)
	opts.OnProgress = func(pr optimize.Progress) {
		reporter.Step(fmt.Sprintf("%d/%d, Kosten %.0f, %s", pr.Iteration, pr.Total, pr.BestCost, pr.Detail))
	}
	st, res := examplan.Solve(prob, opts, keepAssigned)

	reg := prob.Registry()
	total, byC, _ := reg.Cost(st)
	hardVs := reg.HardViolations(st)
	hard := make([]string, 0, len(hardVs))
	for _, v := range hardVs {
		hard = append(hard, fmt.Sprintf("%s: %s %v", v.Constraint, v.Message, v.Refs))
	}
	unplaced := st.UnplacedAncodes()

	result := &ExamScheduleResult{
		Units: len(prob.Units), Unplaced: len(unplaced), UnplacedAncodes: unplaced,
		HardViolations: hard, Cost: total, CostByConstraint: byC,
		Iterations: res.Iterations, StoppedEarly: res.StoppedEarly, Seed: int(seed), Diagnostics: st.Diagnostics(),
	}
	for i := range prob.Units {
		if prob.Units[i].Fixed {
			result.Fixed++
		} else if st.SlotOf[i] >= 0 {
			result.Placed++
		}
	}
	// conflicts of the just-generated schedule (so they can be reviewed/rated even on a
	// dry run, before anything is written)
	slotModel := make(map[[2]int]*model.Slot, len(p.semesterConfig.Slots))
	for _, s := range p.semesterConfig.Slots {
		slotModel[[2]int{s.DayNumber, s.SlotNumber}] = s
	}
	slotByAncode := make(map[int]*model.Slot)
	for i := range prob.Units {
		if st.SlotOf[i] < 0 {
			continue
		}
		if ms := slotModel[[2]int{prob.Slots[st.SlotOf[i]].Day, prob.Slots[st.SlotOf[i]].Slot}]; ms != nil {
			for _, a := range prob.Units[i].Ancodes {
				slotByAncode[a] = ms
			}
		}
	}
	if conflicts, err := p.conflictsFromSlots(ctx, slotByAncode); err == nil {
		result.Conflicts = conflicts
		// diff against the currently saved plan (computed before any write below), so the
		// GUI can show which conflicts are new / gone / worse / better after this run.
		if saved, err := p.ExamScheduleConflicts(ctx); err == nil {
			result.ResolvedConflicts = diffConflictsAgainstSaved(result.Conflicts, saved)
		} else {
			log.Error().Err(err).Msg("cannot compute saved-plan conflicts for diff")
		}
	} else {
		log.Error().Err(err).Msg("cannot compute conflicts of generated schedule")
	}

	d := result.Diagnostics
	reporter.Println(fmt.Sprintf("geplant %d, ungeplant %d, harte Verletzungen %d", result.Placed, result.Unplaced, len(hard)))
	if roomPhase {
		be, ue, bs, us := st.TbauUsage()
		reporter.Println(fmt.Sprintf("T-Bau EXaHM: %d/%d Sitze genutzt, SEB: %d/%d Sitze genutzt", ue, be, us, bs))
	} else {
		reporter.Println(fmt.Sprintf("direkt nacheinander %d, selber Tag %d (%d Studierende), Folgetag %d",
			d.Adjacent, d.SameDay, d.StudentsWithSameDay, d.NextDay))
	}

	if dryRun {
		reporter.StopProgress("Probelauf – nichts geschrieben")
		return result, nil
	}
	if err := p.generationAllowed(ctx, model.PlanningGateExams); err != nil {
		reporter.StopProgressFail(err.Error())
		return result, err
	}
	if len(hard) > 0 {
		reporter.StopProgressFail(fmt.Sprintf("%d harte Verletzungen – nichts geschrieben", len(hard)))
		return result, fmt.Errorf("refusing to write: %d hard violations", len(hard))
	}
	for i := range prob.Units {
		u := &prob.Units[i]
		if u.Fixed {
			continue
		}
		if st.SlotOf[i] < 0 {
			for _, a := range u.Ancodes { // drop any stale entry of a now-unplaced exam
				if err := p.dbClient.RemovePlanEntry(ctx, a); err != nil {
					log.Error().Err(err).Int("ancode", a).Msg("cannot remove stale plan entry")
				}
			}
			continue
		}
		day, slot := prob.Slots[st.SlotOf[i]].Day, prob.Slots[st.SlotOf[i]].Slot
		for _, a := range u.Ancodes {
			if _, err := p.AddExamToSlot(ctx, a, day, slot, true); err != nil {
				reporter.StopProgressFail("Schreiben fehlgeschlagen: " + err.Error())
				return result, fmt.Errorf("cannot write plan entry for %d: %w", a, err)
			}
		}
	}
	result.Written = true
	p.markCondition(ctx, doneCond)
	reporter.StopProgress(fmt.Sprintf("geschrieben: %d Prüfungen", result.Placed))
	return result, nil
}

// FixExamRoomsPhase freezes the EXaHM/SEB room-phase result: every EXaHM/SEB exam that
// has a plan entry is marked PhaseFixed so phase B leaves it untouched (distinct from
// the user's manual Locked). Returns the number of exams fixed.
func (p *Plexams) FixExamRoomsPhase(ctx context.Context) (int, error) {
	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		return 0, err
	}
	planEntries, err := p.PlanEntries(ctx)
	if err != nil {
		return 0, err
	}
	planned := make(map[int]bool, len(planEntries))
	for _, pe := range planEntries {
		planned[pe.Ancode] = true
	}
	n := 0
	for ancode, c := range constraints {
		if c == nil || c.RoomConstraints == nil || (!c.RoomConstraints.Exahm && !c.RoomConstraints.Seb) {
			continue
		}
		if !planned[ancode] {
			continue
		}
		if err := p.dbClient.SetPhaseFixed(ctx, ancode, true); err != nil {
			return n, err
		}
		n++
	}
	p.markCondition(ctx, condExahmSebFixed)
	return n, nil
}

// ResetExamSchedule removes the generated exam schedule (phase B): every plan entry
// that was placed by the generator and is not manually locked, not external / not
// planned by me, and not frozen by the EXaHM/SEB room phase (phaseFixed). The frozen
// EXaHM/SEB placement from phase A is kept — to reset that too, call
// UnfixExamRoomsPhase first (then those entries become resettable). Blocked while the
// exam plan is published. Returns the number of entries removed.
func (p *Plexams) ResetExamSchedule(ctx context.Context) (int, error) {
	if err := p.generationAllowed(ctx, model.PlanningGateExams); err != nil {
		return 0, err
	}
	n, err := p.dbClient.ResetGeneratedPlanEntries(ctx)
	if err != nil {
		return 0, err
	}
	p.unmarkCondition(ctx, condExamScheduleGenerated)
	return n, nil
}

// UnfixExamRoomsPhase clears the phase-A fix on all plan entries (the manual Locked
// stays untouched).
func (p *Plexams) UnfixExamRoomsPhase(ctx context.Context) error {
	if err := p.dbClient.ClearAllPhaseFixed(ctx); err != nil {
		return err
	}
	p.unmarkCondition(ctx, condExahmSebFixed)
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ExamScheduleConstraints returns the read-only description of the hard/soft
// constraints the exam-schedule generator applies.
func (p *Plexams) ExamScheduleConstraints() []optimize.Info {
	prob := &examplan.Problem{W: examplan.DefaultWeights()}
	return prob.Registry().Describe()
}

// repeatForStudent reports whether an exam is (likely) a repeat for a student: the
// exam is flagged a repeater, or the student's semester is higher than the exam's
// (a heuristic via study-group numbers; not fully reliable).
func repeatForStudent(studentSemester int, examRepeater bool, examSemester int) bool {
	if examRepeater {
		return true
	}
	return studentSemester > 0 && examSemester > 0 && studentSemester > examSemester
}

// minGroupSemester returns the smallest semester number found in the exam's groups
// (e.g. "IF2A" -> 2), or 0 if none.
func minGroupSemester(groups []string) int {
	min := 0
	for _, g := range groups {
		if s := semesterOf(g); s > 0 && (min == 0 || s < min) {
			min = s
		}
	}
	return min
}

// semesterOf extracts the first run of digits from a study-group code (e.g. "IF4B"
// -> 4), or 0 if none.
func semesterOf(group string) int {
	start := strings.IndexFunc(group, func(r rune) bool { return r >= '0' && r <= '9' })
	if start < 0 {
		return 0
	}
	end := start
	for end < len(group) && group[end] >= '0' && group[end] <= '9' {
		end++
	}
	n := 0
	for _, c := range group[start:end] {
		n = n*10 + int(c-'0')
	}
	return n
}

// intersectSlots returns the slot indices in both a and b. An empty a means "all slots
// allowed", so the result is b. The result may be empty (no overlap).
func intersectSlots(a, b []int) []int {
	if len(a) == 0 {
		out := make([]int, len(b))
		copy(out, b)
		return out
	}
	set := make(map[int]bool, len(b))
	for _, x := range b {
		set[x] = true
	}
	out := make([]int, 0)
	for _, x := range a {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}

func locationOf(c *model.Constraints) string {
	if c != nil && c.Location != nil {
		return *c.Location
	}
	return ""
}

func firstProgram(e *model.AssembledExam) string {
	for _, pe := range e.PrimussExams {
		if pe != nil && pe.Exam != nil {
			return pe.Exam.Program
		}
	}
	return ""
}

// intersectAllowed intersects the allowed-slot index sets of a same-slot group's
// members. An empty set means "all slots"; the result is nil ("all") when no member
// restricts.
func intersectAllowed(sets [][]int) []int {
	var acc map[int]bool
	for _, s := range sets {
		if len(s) == 0 {
			continue
		}
		m := make(map[int]bool, len(s))
		for _, x := range s {
			m[x] = true
		}
		if acc == nil {
			acc = m
			continue
		}
		for k := range acc {
			if !m[k] {
				delete(acc, k)
			}
		}
	}
	if acc == nil {
		return nil
	}
	out := make([]int, 0, len(acc))
	for k := range acc {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
