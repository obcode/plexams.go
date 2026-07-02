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

// ExamScheduleResult summarizes a Terminplan generation run.
type ExamScheduleResult struct {
	Units            int
	Fixed            int
	Placed           int
	Unplaced         int
	UnplacedAncodes  []int
	HardViolations   []string
	Cost             float64
	CostByConstraint map[string]float64
	Iterations       int
	StoppedEarly     bool
	Written          bool
}

// buildExamPlanProblem assembles the exam-schedule optimization problem from the
// current data: assembled exams to plan (movable), fixed obstacles (locked / external
// / not-planned-by-me), per-student conflict pairs, EXaHM slot capacities and the
// attract pairs (parallel sections / small same-examer exams).
func (p *Plexams) buildExamPlanProblem(ctx context.Context) (*examplan.Problem, error) {
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
			}
		}
	} else {
		log.Warn().Err(err).Msg("cannot read anny bookings; EXaHM slots treated as none")
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
		e         *model.AssembledExam
		fixedSlot int // -1 if movable
		allowed   []int
	}
	rec := make(map[int]*exRec, len(assembled))
	for _, e := range assembled {
		c := constraints[e.Ancode]
		pe := peByAncode[e.Ancode]
		fixed := (c != nil && c.NotPlannedByMe) || (pe != nil && (pe.Locked || pe.ExternalTime != nil))
		if fixed {
			if pe == nil {
				continue // fixed but no slot known → not schedulable, no obstacle
			}
			idx, ok := slotIdx[[2]int{pe.DayNumber, pe.SlotNumber}]
			if !ok {
				continue
			}
			rec[e.Ancode] = &exRec{e: e, fixedSlot: idx}
			continue
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
		rec[e.Ancode] = &exRec{e: e, fixedSlot: -1, allowed: idxs}
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
		for _, a := range members {
			e := rec[a].e
			u.Ancodes = append(u.Ancodes, a)
			u.Seats += e.StudentRegsCount
			if c := constraints[a]; c != nil && c.RoomConstraints != nil && c.RoomConstraints.Exahm {
				u.Exahm = true
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
		u.Allowed = intersectAllowed(allowedSets)
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
		exahm := false
		if c := constraints[a]; c != nil && c.RoomConstraints != nil && c.RoomConstraints.Exahm {
			exahm = true
		}
		units = append(units, examplan.Unit{
			ID: a, Ancodes: []int{a}, Seats: r.e.StudentRegsCount, Exahm: exahm,
			Examer: r.e.ZpaExam.MainExamerID, Module: r.e.ZpaExam.Module, Program: firstProgram(r.e),
			Fixed: true, FixedSlot: r.fixedSlot,
		})
		unitOf[a] = idx
		unitRepeater = append(unitRepeater, r.e.ZpaExam.IsRepeaterExam)
		unitSemester = append(unitSemester, minGroupSemester(r.e.ZpaExam.Groups))
	}

	// --- students / conflict pairs ---
	studentsRaw, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		return nil, err
	}
	w := examplan.DefaultWeights()
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
				if units[a].Fixed && units[b].Fixed {
					continue // both fixed: constant cost, no gradient
				}
				weight := 1.0
				if repeatForStudent(studSem, unitRepeater[a], unitSemester[a]) ||
					repeatForStudent(studSem, unitRepeater[b], unitSemester[b]) {
					weight *= w.RepeatFactor
				}
				pairs = append(pairs, examplan.Pair{A: a, B: b, Weight: weight})
			}
		}
		if len(pairs) > 0 {
			students = append(students, examplan.Student{ID: s.Mtknr, Pairs: pairs})
		}
	}

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

	return examplan.NewProblem(slots, units, students, attract, w), nil
}

// GenerateExamSchedule builds and solves the exam schedule. With dryRun it only
// reports (nothing written); otherwise it writes the non-fixed plan entries (locked /
// external / not-planned-by-me stay untouched).
func (p *Plexams) GenerateExamSchedule(ctx context.Context, dryRun bool, seed int64, iterations int) (*ExamScheduleResult, error) {
	prob, err := p.buildExamPlanProblem(ctx)
	if err != nil {
		return nil, err
	}
	opts := optimize.DefaultOptions()
	opts.Seed = seed
	if iterations > 0 {
		opts.Iterations = iterations
	}
	st, res := examplan.Solve(prob, opts)

	reg := prob.Registry()
	total, byC, _ := reg.Cost(st)
	hardVs := reg.HardViolations(st)
	hard := make([]string, 0, len(hardVs))
	for _, v := range hardVs {
		hard = append(hard, fmt.Sprintf("%s: %s %v", v.Constraint, v.Message, v.Refs))
	}
	unplaced := st.UnplacedAncodes()

	result := &ExamScheduleResult{
		Units: len(prob.Units), Placed: 0, Unplaced: len(unplaced), UnplacedAncodes: unplaced,
		HardViolations: hard, Cost: total, CostByConstraint: byC,
		Iterations: res.Iterations, StoppedEarly: res.StoppedEarly,
	}
	for i := range prob.Units {
		if prob.Units[i].Fixed {
			result.Fixed++
		} else if st.SlotOf[i] >= 0 {
			result.Placed++
		}
	}

	if !dryRun {
		if len(hard) > 0 {
			return result, fmt.Errorf("refusing to write: %d hard violations", len(hard))
		}
		for i := range prob.Units {
			u := &prob.Units[i]
			if u.Fixed || st.SlotOf[i] < 0 {
				continue
			}
			day, slot := prob.Slots[st.SlotOf[i]].Day, prob.Slots[st.SlotOf[i]].Slot
			for _, a := range u.Ancodes {
				if _, err := p.AddExamToSlot(ctx, a, day, slot, true); err != nil {
					return result, fmt.Errorf("cannot write plan entry for %d: %w", a, err)
				}
			}
		}
		result.Written = true
	}
	return result, nil
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
