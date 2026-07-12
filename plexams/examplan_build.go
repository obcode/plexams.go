package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/conflictcalc"
	"github.com/obcode/plexams.go/plexams/examplan"
	"github.com/obcode/plexams.go/plexams/optimize"
	"github.com/obcode/plexams.go/plexams/repeatcalc"
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
	// ExahmNtaAncodes are the EXaHM/SEB exams that carry an NTA. Their NTA time extension is
	// deliberately not gated against the Anny booking window (the NTA student is seated in a
	// separate NTA room booked later, at room planning); this list is the reminder to book
	// that room. See buildExamPlanProblem.
	ExahmNtaAncodes []int
	// UnplacedReasons explains, per unplaced exam, why it could not be scheduled — a
	// structural reason (no allowed MUC.DAI time, no EXaHM/SEB booking covers its window) or,
	// when it did have candidate slots, that none stayed free in this run.
	UnplacedReasons []*UnplacedExamReason
}

// UnplacedExamReason is the human-readable reason a single exam ended up unplaced.
type UnplacedExamReason struct {
	Ancode int
	Reason string
}

// reasons an exam can never be placed, decided during problem construction (before solving).
const (
	unplaceableNoMucDaiSlot   = "keine passende MUC.DAI-Zeit (Fachtermine ∩ MUC.DAI-Zeiten leer)"
	unplaceableNoExahmBooking = "keine EXaHM/SEB-Buchung deckt das Prüfungsfenster (Prüfungsdauer + Puffer)"
	// %s is filled with the concrete window bound (e.g. "nicht vor 10:00" / "nicht nach 14:00").
	unplaceableOutsideTimeWindow = "kein erlaubter Slot im Zeitfenster (%s); ggf. Enforcement auf SOFT stellen"
)

// examPlanBuildInfo carries diagnostics from problem construction that the caller surfaces in
// the run report; they are not inputs to the solver and so are kept off the examplan.Problem.
type examPlanBuildInfo struct {
	// exahmNtaAncodes: EXaHM/SEB exams with an NTA (reminder to book an NTA room).
	exahmNtaAncodes []int
	// unplaceableReason maps an ancode that can NEVER be placed (its allowed set is the
	// unplaceable sentinel) to why. Exams unplaced only because no candidate slot stayed free
	// are not in here — the caller reports those with a capacity/conflict fallback.
	unplaceableReason map[int]string
}

// buildExamPlanProblem assembles the exam-schedule optimization problem from the
// current data: assembled exams to plan (movable), fixed obstacles (locked / external
// / not-planned-by-me), per-student conflict pairs, EXaHM slot capacities and the
// attract pairs (parallel sections / small same-examer exams).
func (p *Plexams) buildExamPlanProblem(ctx context.Context, applyRatings, roomPhase bool) (*examplan.Problem, *examPlanBuildInfo, error) {
	sc := p.semesterConfig
	if sc == nil {
		return nil, nil, fmt.Errorf("no semester config loaded")
	}

	// --- slots ---
	slotStarts := make([]time.Time, 0, len(sc.Slots))
	idxByStart := make(map[time.Time]int, len(sc.Slots))
	slots := make([]examplan.Slot, 0, len(sc.Slots))
	for _, s := range sc.Slots {
		idxByStart[s.Starttime] = len(slots)
		slotStarts = append(slotStarts, s.Starttime)
		slots = append(slots, examplan.Slot{
			SlotRef: examplan.SlotRef{Start: s.Starttime},
			// Seats caps how many students may be examined at this start time. The
			// configurable per-time capacity (SemesterConfig.MaxSeatsPerSlot); 0 = unlimited.
			Seats: sc.MaxSeatsPerSlot,
		})
	}
	if booked, err := p.annyBookedByTime(ctx, slotStarts); err == nil {
		for start, sb := range booked {
			if idx, ok := idxByStart[start]; ok && sb != nil {
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
		return nil, nil, err
	}
	planEntries, err := p.PlanEntries(ctx)
	if err != nil {
		return nil, nil, err
	}
	peByAncode := make(map[int]*model.PlanEntry, len(planEntries))
	for _, pe := range planEntries {
		peByAncode[pe.Ancode] = pe
	}
	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		return nil, nil, err
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
		foreign := (c != nil && c.NotPlannedByMe) || (pe != nil && pe.External) || e.Ancode >= externalAncodeBase
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
			if pe == nil || pe.Starttime == nil {
				continue // fixed but no time known → not schedulable, no obstacle
			}
			idx, ok := slotIndexAt(slotStarts, *pe.Starttime)
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
			return nil, nil, fmt.Errorf("allowed slots for %d: %w", e.Ancode, err)
		}
		idxs := make([]int, 0, len(allowedSlots))
		for _, s := range allowedSlots {
			if idx, ok := idxByStart[s.Starttime]; ok {
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
			if s := repeatcalc.MinGroupSemester(e.ZpaExam.Groups); s > 0 && (minSem == 0 || s < minSem) {
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
			u.Allowed = examplan.IntersectAllowed(allowedSets)
		}
		// warm-start slot = this exam's current plan entry (if any)
		u.StartSlot = -1
		if pe := peByAncode[members[0]]; pe != nil && pe.Starttime != nil {
			if idx, ok := slotIndexAt(slotStarts, *pe.Starttime); ok {
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
		unitSemester = append(unitSemester, repeatcalc.MinGroupSemester(r.e.ZpaExam.Groups))
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
		return nil, nil, err
	}

	// MUC.DAI restriction: any exam a MUC.DAI-program student (DE/GS/ID) is registered
	// for must be scheduled in a MUC.DAI slot — this includes normal ZPA exams (e.g.
	// 134/118) that a MUC.DAI student takes, not only the external MUC.DAI exams.
	mucDaiProg := make(map[string]bool)
	for _, prog := range p.mucdaiProgramNames(ctx) {
		mucDaiProg[prog] = true
	}
	mucDaiSlotIdx := make([]int, 0)
	for _, t := range sc.MucDaiAllowedTimes {
		if idx, ok := slotIndexAt(slotStarts, *t); ok {
			mucDaiSlotIdx = append(mucDaiSlotIdx, idx)
		}
	}
	// unplaceableReason collects, per ancode, why it can never be placed (its allowed set
	// becomes the -1 sentinel). Reported by the caller for the unplaced exams.
	unplaceableReason := make(map[int]string)
	if len(mucDaiProg) > 0 && len(mucDaiSlotIdx) > 0 {
		mucDaiUnit := make(map[int]bool)
		for _, s := range studentsRaw {
			for _, rwp := range s.RegsWithProgram {
				if mucDaiProg[rwp.Program] {
					if u, ok := unitOf[rwp.ZpaAncode]; ok && !units[u].Fixed {
						mucDaiUnit[u] = true
					}
				}
			}
		}
		for u := range mucDaiUnit {
			inter := examplan.IntersectSlots(units[u].Allowed, mucDaiSlotIdx)
			if len(inter) == 0 {
				inter = []int{-1} // no MUC.DAI slot fits its other constraints → unplaceable (reported)
				for _, a := range units[u].Ancodes {
					unplaceableReason[a] = unplaceableNoMucDaiSlot
				}
			}
			units[u].Allowed = inter
		}
	}

	// solver weights from the (GUI-editable) generation config, seeded with the tuned
	// defaults; TimeOfDay is set below from the per-semester start-time severity.
	genCfg, err := p.GenerationConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	w := examScheduleWeights(genCfg)
	if roomPhase {
		// Phase A: main goal is to fill the booked T-building rooms with EXaHM/SEB;
		// even distribution over all slots is off (we want concentration in T-Bau), and
		// interior-hole avoidance is meaningless for this subset (phase B fills the day).
		w.TbauFill = 10000
		w.SlotLoad = 0
		w.Hole = 0
	}
	// start-time window (semester-dependent): resolve the per-slot soft severity, the weight
	// and — for HARD enforcement — the per-slot "outside the window" flags. EXaHM/SEB exams
	// are exempt (climate-controlled T-Bau rooms) and are handled per-unit in the solver /
	// skipped by the HARD domain filter below; in phase A every movable exam is EXaHM/SEB, so
	// the window is inert there.
	timeSpec := p.slotTimeSpec(ctx, slots)
	w.TimeOfDay = timeSpec.weight
	students := make([]examplan.Student, 0, len(studentsRaw))
	for _, s := range studentsRaw {
		seen := make(map[int]bool)
		list := make([]int, 0, len(s.ZpaAncodes))
		for _, ancode := range s.ZpaAncodes {
			if ui, ok := unitOf[ancode]; ok && !seen[ui] {
				seen[ui] = true
				list = append(list, ui)
			}
		}
		if len(list) < 2 {
			continue
		}
		sort.Ints(list)
		studSem := repeatcalc.SemesterOf(s.Group)
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
				isRepeat := repeatcalc.RepeatForStudent(studSem, unitRepeater[a], unitSemester[a]) ||
					repeatcalc.RepeatForStudent(studSem, unitRepeater[b], unitSemester[b])
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
	attract := examplan.AttractPairs(units, smallExamThreshold)

	// --- consecutive-exam gap (hard): a student needs a travel/break buffer between two
	// of their exams; an NTA time extension eats into it. Two conflicting exams of a
	// student must not overlap in time — the later may only start once the earlier one's
	// occupied time (its duration, extended for that student's NTA) plus the buffer has
	// passed. hardSep[{a,b}] is that minimum separation (minutes) from a's start to b's,
	// aggregated over the students sharing the pair.
	gapMin := sc.ExamGapMinutes
	if gapMin <= 0 {
		gapMin = defaultExamGapMinutes
	}
	unitBaseDur := make(map[int]int)       // unit -> exam duration
	ntaExt := make(map[int]map[string]int) // unit -> mtknr -> NTA-extended duration
	unitPreMin := make(map[int]int)        // unit -> Vorlauf minutes (EXaHM/SEB default 30)
	unitPostMin := make(map[int]int)       // unit -> Nachlauf minutes (EXaHM/SEB default 30, larger if extended)
	for _, e := range assembled {
		ui, ok := unitOf[e.Ancode]
		if !ok || e.ZpaExam == nil {
			continue
		}
		if e.ZpaExam.Duration > unitBaseDur[ui] {
			unitBaseDur[ui] = e.ZpaExam.Duration
		}
		// EXaHM/SEB exams run in booked T-building rooms and need the larger 30-min default
		// setup/teardown (overridable per exam); regular rooms keep the 15-min turnaround.
		c := constraints[e.Ancode]
		var pre, post time.Duration
		if c != nil && c.RoomConstraints != nil && (c.RoomConstraints.Exahm || c.RoomConstraints.Seb) {
			pre, post = exahmRoomBuffers(c)
		} else {
			pre, post = roomBuffers(c)
		}
		if int(pre.Minutes()) > unitPreMin[ui] {
			unitPreMin[ui] = int(pre.Minutes())
		}
		if int(post.Minutes()) > unitPostMin[ui] {
			unitPostMin[ui] = int(post.Minutes())
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
	// occFor: the student's occupied minutes for an exam (its duration, extended for that
	// student's NTA).
	occFor := func(unit int, mtknr string) int {
		dur := unitBaseDur[unit]
		if m := ntaExt[unit]; m != nil {
			if ext, ok := m[mtknr]; ok && ext > dur {
				dur = ext
			}
		}
		return dur
	}
	// build the per-pair minimum separations from the (already conflict-filtered) student
	// pairs, so hardSep covers exactly the same pairs as the solver's hard conflicts.
	hardSep := make(map[[2]int]int)
	for _, s := range students {
		for _, pr := range s.Pairs {
			if sep := occFor(pr.A, s.ID) + gapMin; sep > hardSep[[2]int{pr.A, pr.B}] {
				hardSep[[2]int{pr.A, pr.B}] = sep
			}
			if sep := occFor(pr.B, s.ID) + gapMin; sep > hardSep[[2]int{pr.B, pr.A}] {
				hardSep[[2]int{pr.B, pr.A}] = sep
			}
		}
	}

	// EXaHM room-window restriction: an EXaHM exam may only occupy a slot where a booked
	// EXaHM room's Anny window actually covers its exam window (the BASE exam duration plus
	// the setup/teardown buffer). This is the schedule-generator counterpart of the
	// Vorplanung gate, so phase A (and any movable EXaHM in phase B) never places an exam
	// into a booking too small for it (e.g. Embedded Computing, 120 min + 60/60, into a room
	// booked only 16:00–18:30). A unit no booking can host gets the unplaceable sentinel and
	// is reported as unplaced.
	//
	// NTA time extensions are deliberately NOT counted against this window: at Terminplanung
	// time the NTA rooms (e.g. T3.021) are not booked yet — an NTA student is seated in a
	// separate NTA room during room planning, not in the shared EXaHM booking — and the
	// modest extension is anyway absorbed by the teardown buffer (a 90-min exam + 10 % NTA
	// = 99 min still fits an 08:00–10:30 booking, leaving 21 of the 30 min Nachlauf). Adding
	// the NTA on top would drop otherwise-fine slots. EXaHM exams that carry an NTA are
	// collected instead so the operator is reminded to book an NTA room.
	exahmIntervals, err := p.bookedExahmIntervals(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("cannot read anny bookings for EXaHM window restriction; EXaHM exams left ungated")
		exahmIntervals = nil
	}
	blockDur := slotBlockDuration(sc.Starttimes)
	allSlotIdx := make([]int, len(slots))
	for i := range slots {
		allSlotIdx[i] = i
	}
	restrictedUnplaceable := make([]int, 0)
	exahmWithNTA := make([]int, 0)
	for u := range units {
		if !units[u].Exahm || units[u].Fixed {
			continue
		}
		if len(ntaExt[u]) > 0 {
			exahmWithNTA = append(exahmWithNTA, units[u].ID)
		}
		dur := time.Duration(unitBaseDur[u]) * time.Minute
		if dur == 0 {
			dur = blockDur
		}
		preMin, postMin := unitPreMin[u], unitPostMin[u]
		if preMin == 0 {
			preMin = int(exahmDefaultBuffer.Minutes())
		}
		if postMin == 0 {
			postMin = int(exahmDefaultBuffer.Minutes())
		}
		pre := time.Duration(preMin) * time.Minute
		post := time.Duration(postMin) * time.Minute
		base := units[u].Allowed
		if len(base) == 0 {
			base = allSlotIdx // empty Allowed means "all slots"; make it explicit before filtering
		}
		hadRealSlots := false
		covered := make([]int, 0, len(base))
		for _, idx := range base {
			if idx < 0 || idx >= len(slots) {
				continue
			}
			hadRealSlots = true
			// enough booked EXaHM seats from rooms that actually cover this exam's window
			// (rooms booked too short don't count), not merely "some room covers it".
			if exahmWindowSeats(exahmIntervals, true, slots[idx].Start, dur, pre, post) >= units[u].Seats {
				covered = append(covered, idx)
			}
		}
		if len(covered) == 0 {
			covered = []int{-1} // no booking covers the window → unplaceable (reported as unplaced)
			// Only blame the booking window when the unit still had real candidate slots here;
			// if it was already the -1 sentinel (e.g. no MUC.DAI time), keep that upstream reason.
			if hadRealSlots {
				restrictedUnplaceable = append(restrictedUnplaceable, units[u].ID)
				for _, a := range units[u].Ancodes {
					unplaceableReason[a] = unplaceableNoExahmBooking
				}
			}
		}
		units[u].Allowed = covered
	}
	if len(restrictedUnplaceable) > 0 {
		sort.Ints(restrictedUnplaceable)
		log.Warn().Ints("ancodes", restrictedUnplaceable).Int("count", len(restrictedUnplaceable)).
			Msg("EXaHM exams have no Anny booking large enough for their window — left unplaced")
	}
	if len(exahmWithNTA) > 0 {
		sort.Ints(exahmWithNTA)
		log.Info().Ints("ancodes", exahmWithNTA).Int("count", len(exahmWithNTA)).
			Msg("EXaHM exams with NTA students — book an NTA room at room planning (not gated here)")
	}

	// Start-time window (HARD enforcement): restrict every movable, NON-exempt exam to the
	// slots inside its semester window (winter: start ≥ earliest; summer: start ≤ latest).
	// EXaHM/SEB exams are exempt (climate-controlled T-Bau rooms); in phase A every movable
	// exam is EXaHM/SEB, so this loop is a no-op there. A unit left with no in-window slot
	// gets the unplaceable sentinel and is reported as unplaced (the rest of the plan is still
	// written). SOFT enforcement sets timeSpec.hard = false and skips this entirely (the
	// window is then a solver penalty instead). Same shape as the EXaHM-window filter above.
	if timeSpec.hard && len(timeSpec.forbidden) == len(slots) {
		windowUnplaceable := make([]int, 0)
		for u := range units {
			if units[u].Fixed || units[u].Exahm || units[u].Seb {
				continue
			}
			base := units[u].Allowed
			if len(base) == 0 {
				base = allSlotIdx // empty Allowed means "all slots"; make it explicit before filtering
			}
			hadRealSlots := false
			inWindow := make([]int, 0, len(base))
			for _, idx := range base {
				if idx < 0 || idx >= len(slots) {
					continue
				}
				hadRealSlots = true
				if !timeSpec.forbidden[idx] {
					inWindow = append(inWindow, idx)
				}
			}
			if len(inWindow) == 0 {
				inWindow = []int{-1} // no in-window slot → unplaceable (reported as unplaced)
				if hadRealSlots {
					windowUnplaceable = append(windowUnplaceable, units[u].ID)
					for _, a := range units[u].Ancodes {
						unplaceableReason[a] = fmt.Sprintf(unplaceableOutsideTimeWindow, timeWindowBoundText(timeSpec))
					}
				}
			}
			units[u].Allowed = inWindow
		}
		if len(windowUnplaceable) > 0 {
			sort.Ints(windowUnplaceable)
			log.Warn().Ints("ancodes", windowUnplaceable).Int("count", len(windowUnplaceable)).
				Msg("exams have no slot inside the start-time window — left unplaced (set enforcement to SOFT to override)")
		}
	}

	// Room overrun (time-based): an EXaHM exam with an extended Nachlauf keeps its booked
	// T-building rooms occupied past its own start time; the later slots whose exam window
	// it reaches must count it against their booked EXaHM seats too. Compute, per such unit
	// and candidate slot, the later same-day slots it overruns into, from the absolute slot
	// start times (no slot-number arithmetic). Empty for every exam on the ordinary
	// turnaround, so the solver's capacity check is unchanged for them. The turnaround is the
	// shared EXaHM buffer (30 min = 15 each side), so a default EXaHM exam does NOT overrun;
	// only a widened Nachlauf (e.g. 60) does.
	turnaround := int(exahmDefaultBuffer.Minutes())
	overrun := make(map[[2]int][]int)
	for u := range units {
		if !units[u].Exahm || unitPostMin[u] <= turnaround {
			continue
		}
		holdAfterStart := unitBaseDur[u] + unitPostMin[u] // room occupied until start + this
		for s := range slots {
			var targets []int
			for t := range slots {
				if t == s || !sameCalendarDay(slots[t].Start, slots[s].Start) {
					continue
				}
				d := int(slots[t].Start.Sub(slots[s].Start).Minutes())
				// t's exam needs the room from Start[t]-turnaround; u releases it at
				// Start[s]+holdAfterStart. They compete iff the former is earlier.
				if d > 0 && d-turnaround < holdAfterStart {
					targets = append(targets, t)
				}
			}
			if len(targets) > 0 {
				overrun[[2]int{u, s}] = targets
			}
		}
	}

	prob := examplan.NewProblem(slots, units, students, attract, w)
	prob.SetTimeSeverity(timeSpec.severity)
	prob.SetTimeWindow(timeSpec.mode, timeSpec.earliestMin, timeSpec.latestMin)
	prob.SetHardSeparations(hardSep)
	prob.SetOverrunTargets(overrun)
	return prob, &examPlanBuildInfo{exahmNtaAncodes: exahmWithNTA, unplaceableReason: unplaceableReason}, nil
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
	prob, buildInfo, err := p.buildExamPlanProblem(ctx, !ignoreRatings, roomPhase)
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
	if n := prob.NumHardSeparations(); n > 0 {
		reporter.Println(fmt.Sprintf("%d Zwischenzeit-Sperren berücksichtigt (Zeit-Überlappung, inkl. NTA)", n))
	}
	// EXaHM/SEB exams with an NTA are placed on their base exam duration (the NTA is not
	// gated against the Anny booking window — see buildExamPlanProblem). Remind the operator
	// to book a separate NTA room for them during room planning.
	if len(buildInfo.exahmNtaAncodes) > 0 {
		reporter.Println(fmt.Sprintf("Hinweis: %d EXaHM/SEB-Prüfung(en) mit NTA — bei der Raumplanung NTA-Raum buchen: %v",
			len(buildInfo.exahmNtaAncodes), buildInfo.exahmNtaAncodes))
	}
	// For the ordinary Terminplan run (not the EXaHM/SEB room phase) warn if exams planned
	// by another faculty still miss their Termin: those exams cannot constrain the plan yet,
	// so conflicts with them are invisible in this run.
	if !roomPhase {
		if missing, err := p.unscheduledOtherFacultyExams(ctx); err != nil {
			log.Error().Err(err).Msg("cannot check whether other-faculty exams are scheduled")
		} else if len(missing) > 0 {
			reporter.Warnf("Achtung: %d Prüfung(en) anderer FKs noch ohne Termin – Konflikte mit ihnen werden in diesem Lauf nicht berücksichtigt: %v",
				len(missing), missing)
		}
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

	// per-exam reason why it stayed unplaced: a structural reason decided at problem
	// construction (no MUC.DAI time / no EXaHM booking covers the window), else it had
	// candidate slots but none stayed free in this run (capacity / conflicts).
	var unplacedReasons []*UnplacedExamReason
	if len(unplaced) > 0 {
		unitOfAncode := make(map[int]int, len(prob.Units))
		for i := range prob.Units {
			for _, a := range prob.Units[i].Ancodes {
				unitOfAncode[a] = i
			}
		}
		for _, a := range unplaced {
			reason := buildInfo.unplaceableReason[a]
			if reason == "" {
				n := -1
				if i, ok := unitOfAncode[a]; ok {
					n = len(prob.Units[i].Allowed)
				}
				if n == 0 { // empty Allowed = all slots were allowed
					reason = "alle Slots zulässig, aber im Lauf keiner frei (Kapazität/Konflikte)"
				} else {
					reason = fmt.Sprintf("%d zulässige(r) Slot(s), aber im Lauf keiner frei (Kapazität/Konflikte)", n)
				}
			}
			unplacedReasons = append(unplacedReasons, &UnplacedExamReason{Ancode: a, Reason: reason})
		}
	}

	result := &ExamScheduleResult{
		Units: len(prob.Units), Unplaced: len(unplaced), UnplacedAncodes: unplaced,
		HardViolations: hard, Cost: total, CostByConstraint: byC,
		Iterations: res.Iterations, StoppedEarly: res.StoppedEarly, Seed: int(seed), Diagnostics: st.Diagnostics(),
		ExahmNtaAncodes: buildInfo.exahmNtaAncodes, UnplacedReasons: unplacedReasons,
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
	slotModel := make(map[time.Time]*model.Slot, len(p.semesterConfig.Slots))
	for _, s := range p.semesterConfig.Slots {
		slotModel[s.Starttime] = s
	}
	slotByAncode := make(map[int]*model.Slot)
	for i := range prob.Units {
		if st.SlotOf[i] < 0 {
			continue
		}
		if ms := slotModel[prob.Slots[st.SlotOf[i]].Start]; ms != nil {
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
			result.ResolvedConflicts = conflictcalc.DiffAgainstSaved(result.Conflicts, saved)
		} else {
			log.Error().Err(err).Msg("cannot compute saved-plan conflicts for diff")
		}
	} else {
		log.Error().Err(err).Msg("cannot compute conflicts of generated schedule")
	}

	d := result.Diagnostics
	reporter.Println(fmt.Sprintf("geplant %d, ungeplant %d, harte Verletzungen %d", result.Placed, result.Unplaced, len(hard)))
	// list the concrete hard violations (they block the write): both to the stream so the
	// GUI shows them and to the server log so they are visible without the GUI report.
	for _, h := range hard {
		reporter.Println("  harte Verletzung: " + h)
		log.Warn().Bool("roomPhase", roomPhase).Str("violation", h).Msg("exam schedule hard violation")
	}
	// per-exam reason for every unplaced exam, so it is clear why the solver left it out.
	for _, ur := range result.UnplacedReasons {
		reporter.Println(fmt.Sprintf("  %d nicht geplant: %s", ur.Ancode, ur.Reason))
		log.Warn().Int("ancode", ur.Ancode).Str("reason", ur.Reason).Msg("exam left unplaced")
	}
	if roomPhase {
		be, ue, bs, us := st.TbauUsage()
		reporter.Println(fmt.Sprintf("T-Bau EXaHM: %d/%d Sitze genutzt, SEB: %d/%d Sitze genutzt", ue, be, us, bs))
	} else {
		reporter.Println(fmt.Sprintf("direkt nacheinander %d, selber Tag %d (%d Studierende), Folgetag %d",
			d.Adjacent, d.SameDay, d.StudentsWithSameDay, d.NextDay))
		if d.InteriorHoles > 0 {
			reporter.Println(fmt.Sprintf("freie Slots mitten am Tag: %d (ungünstig für die Aufsichtenplanung)", d.InteriorHoles))
		}
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
		start := prob.Slots[st.SlotOf[i]].Start
		for _, a := range u.Ancodes {
			if _, err := p.SetExamTime(ctx, a, start); err != nil {
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

// exahmSebPlanFixState returns, for every EXaHM/SEB exam that currently has a plan entry
// (the set the room phase freezes), whether that entry is already frozen (PhaseFixed). It is
// the single source of truth for "which exams the EXaHM/SEB room phase covers", shared by
// FixExamRoomsPhase (what to freeze) and ExamRoomsPhaseState (the GUI summary), so the two
// can never disagree.
func (p *Plexams) exahmSebPlanFixState(ctx context.Context) (map[int]bool, error) {
	constraints, err := p.ConstraintsMap(ctx)
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
	fixedByAncode := make(map[int]bool)
	for ancode, c := range constraints {
		if c == nil || c.RoomConstraints == nil || (!c.RoomConstraints.Exahm && !c.RoomConstraints.Seb) {
			continue
		}
		pe := peByAncode[ancode]
		if pe == nil {
			continue // EXaHM/SEB but not planned → not part of the room phase yet
		}
		fixedByAncode[ancode] = pe.PhaseFixed
	}
	return fixedByAncode, nil
}

// FixExamRoomsPhase freezes the EXaHM/SEB room-phase result: every EXaHM/SEB exam that
// has a plan entry is marked PhaseFixed so phase B leaves it untouched (distinct from
// the user's manual Locked). Returns the number of exams fixed.
func (p *Plexams) FixExamRoomsPhase(ctx context.Context) (int, error) {
	state, err := p.exahmSebPlanFixState(ctx)
	if err != nil {
		return 0, err
	}
	ancodes := make([]int, 0, len(state))
	for a := range state {
		ancodes = append(ancodes, a)
	}
	sort.Ints(ancodes) // deterministic write order
	n := 0
	for _, ancode := range ancodes {
		if err := p.dbClient.SetPhaseFixed(ctx, ancode, true); err != nil {
			return n, err
		}
		n++
	}
	p.markCondition(ctx, condExahmSebFixed)
	return n, nil
}

// ExamRoomsPhaseState summarizes the EXaHM/SEB room phase for the GUI: how many EXaHM/SEB
// exams are planned and how many of those are frozen (PhaseFixed) for phase B. Drives the
// fixed-state display and the "not fixed" warning before phase-B generation.
func (p *Plexams) ExamRoomsPhaseState(ctx context.Context) (*model.ExamRoomsPhaseState, error) {
	state, err := p.exahmSebPlanFixState(ctx)
	if err != nil {
		return nil, err
	}
	planned, fixed := 0, 0
	for _, isFixed := range state {
		planned++
		if isFixed {
			fixed++
		}
	}
	return &model.ExamRoomsPhaseState{
		Planned:  planned,
		Fixed:    fixed,
		AllFixed: planned > 0 && fixed == planned,
	}, nil
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

// slotIndexAt returns the index i where slotStarts[i] equals t (via time.Equal, robust
// for BSON-decoded plan-entry times whose internal representation may differ from the
// config-built grid times), and whether such an index exists.
func slotIndexAt(slotStarts []time.Time, t time.Time) (int, bool) {
	for i := range slotStarts {
		if slotStarts[i].Equal(t) {
			return i, true
		}
	}
	return 0, false
}

// sameCalendarDay reports whether two absolute times fall on the same local calendar
// day (used for the room-overrun check now that slots carry no day ordinal).
func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// ExamScheduleConstraints returns the read-only description of the hard/soft
// constraints the exam-schedule generator applies.
func (p *Plexams) ExamScheduleConstraints() []optimize.Info {
	prob := &examplan.Problem{W: examplan.DefaultWeights()}
	return prob.Registry().Describe()
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
