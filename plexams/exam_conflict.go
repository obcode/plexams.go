package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/conflictcalc"
	"github.com/obcode/plexams.go/plexams/repeatcalc"
)

// StudentConflictDecisions returns all stored explicit per-student decisions.
func (p *Plexams) StudentConflictDecisions(ctx context.Context) ([]*model.StudentConflictDecision, error) {
	return p.dbClient.StudentConflictDecisions(ctx)
}

// SetStudentConflictDecision sets an explicit per-student decision: ACCEPT drops that
// student's proximity penalty (same-slot stays hard); VETO forces the conflict to count
// at full weight, overriding an automatic repeat down-weighting.
func (p *Plexams) SetStudentConflictDecision(ctx context.Context, ancode1, ancode2 int, mtknr string, decision model.ConflictDecision) (bool, error) {
	if ancode1 == ancode2 {
		return false, fmt.Errorf("cannot decide an exam against itself")
	}
	if mtknr == "" {
		return false, fmt.Errorf("mtknr required")
	}
	if !decision.IsValid() {
		return false, fmt.Errorf("invalid decision %q", decision)
	}
	a, b := conflictcalc.NormPair(ancode1, ancode2)
	if err := p.dbClient.UpsertDecision(ctx, a, b, mtknr, string(decision)); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveStudentConflictDecision clears an explicit decision (back to automatic handling).
func (p *Plexams) RemoveStudentConflictDecision(ctx context.Context, ancode1, ancode2 int, mtknr string) (bool, error) {
	a, b := conflictcalc.NormPair(ancode1, ancode2)
	return p.dbClient.DeleteDecision(ctx, a, b, mtknr)
}

// ExamsCanShareSlot returns the declared can-share-slot pairs with display info.
func (p *Plexams) ExamsCanShareSlot(ctx context.Context) ([]*model.ExamPair, error) {
	pairs, err := p.dbClient.CanShareSlotPairs(ctx)
	if err != nil {
		return nil, err
	}
	info := p.examInfoMap(ctx)
	out := make([]*model.ExamPair, 0, len(pairs))
	for _, pr := range pairs {
		out = append(out, examPair(pr[0], pr[1], info))
	}
	return out, nil
}

// SetExamsCanShareSlot declares that two exams may share a slot.
func (p *Plexams) SetExamsCanShareSlot(ctx context.Context, ancode1, ancode2 int) (bool, error) {
	if ancode1 == ancode2 {
		return false, fmt.Errorf("an exam always shares its own slot")
	}
	a, b := conflictcalc.NormPair(ancode1, ancode2)
	if err := p.dbClient.UpsertCanShareSlot(ctx, a, b); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveExamsCanShareSlot undeclares a can-share-slot pair.
func (p *Plexams) RemoveExamsCanShareSlot(ctx context.Context, ancode1, ancode2 int) (bool, error) {
	a, b := conflictcalc.NormPair(ancode1, ancode2)
	return p.dbClient.DeleteCanShareSlot(ctx, a, b)
}

// CanShareSlotSuggestions returns parallel-section candidates (same module+program,
// different examer) not yet declared as can-share-slot.
func (p *Plexams) CanShareSlotSuggestions(ctx context.Context) ([]*model.ExamPair, error) {
	assembled, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		return nil, err
	}
	existing := make(map[[2]int]bool)
	if pairs, err := p.dbClient.CanShareSlotPairs(ctx); err == nil {
		for _, pr := range pairs {
			existing[[2]int{pr[0], pr[1]}] = true
		}
	}
	// pairs already forced into the same slot by a sameSlot constraint need no
	// "may share a slot" suggestion — they must be together anyway.
	sameSlotRoot := p.sameSlotGroups(ctx)

	byKey := make(map[string][]*model.AssembledExam)
	for _, e := range assembled {
		key := e.ZpaExam.Module + "|" + firstProgram(e)
		byKey[key] = append(byKey[key], e)
	}
	info := p.examInfoMap(ctx)
	out := make([]*model.ExamPair, 0)
	for _, list := range byKey {
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if list[i].ZpaExam.MainExamerID == list[j].ZpaExam.MainExamerID {
					continue
				}
				a, b := conflictcalc.NormPair(list[i].Ancode, list[j].Ancode)
				if existing[[2]int{a, b}] {
					continue
				}
				if r, ok := sameSlotRoot[a]; ok && r == sameSlotRoot[b] {
					continue // already forced same slot via sameSlot constraint
				}
				out = append(out, examPair(a, b, info))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ancode1 != out[j].Ancode1 {
			return out[i].Ancode1 < out[j].Ancode1
		}
		return out[i].Ancode2 < out[j].Ancode2
	})
	return out, nil
}

// sameSlotGroups returns, per ancode that is in a sameSlot constraint, a group root
// ancode, so two ancodes with the same root must share a slot.
func (p *Plexams) sameSlotGroups(ctx context.Context) map[int]int {
	root := make(map[int]int)
	var find func(int) int
	find = func(x int) int {
		if r, ok := root[x]; ok && r != x {
			root[x] = find(r)
			return root[x]
		}
		return x
	}
	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		return root
	}
	for ancode, c := range constraints {
		if c == nil {
			continue
		}
		for _, other := range c.SameSlot {
			if _, ok := root[ancode]; !ok {
				root[ancode] = ancode
			}
			if _, ok := root[other]; !ok {
				root[other] = other
			}
			root[find(ancode)] = find(other)
		}
	}
	// path-compress everything to a stable root
	for k := range root {
		root[k] = find(k)
	}
	return root
}

type examInfo struct {
	module   string
	examer   string
	groups   []string
	repeater bool
	minSem   int
}

func (p *Plexams) examInfoMap(ctx context.Context) map[int]examInfo {
	m := make(map[int]examInfo)
	assembled, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		return m
	}
	for _, e := range assembled {
		groups := e.ZpaExam.Groups
		if groups == nil {
			groups = []string{}
		}
		m[e.Ancode] = examInfo{
			module: e.ZpaExam.Module, examer: e.ZpaExam.MainExamer, groups: groups,
			repeater: e.ZpaExam.IsRepeaterExam, minSem: repeatcalc.MinGroupSemester(e.ZpaExam.Groups),
		}
	}
	return m
}

func examPair(a, b int, info map[int]examInfo) *model.ExamPair {
	return &model.ExamPair{
		Ancode1: a, Module1: info[a].module, MainExamer1: info[a].examer,
		Ancode2: b, Module2: info[b].module, MainExamer2: info[b].examer,
	}
}

// ExamScheduleConflicts computes the conflicts of the CURRENT plan (from the plan
// entries): per student, pairs of their exams that ended up close in time. It
// aggregates by exam pair (worst proximity, number of affected students) and annotates
// each with any stored rating and whether the pair is declared can-share-slot. This is
// the list the user rates to steer the next generation.
func (p *Plexams) ExamScheduleConflicts(ctx context.Context) ([]*model.ExamScheduleConflict, error) {
	planEntries, err := p.PlanEntries(ctx)
	if err != nil {
		return nil, err
	}
	slotModel := make(map[[2]int]*model.Slot, len(p.semesterConfig.Slots))
	for _, s := range p.semesterConfig.Slots {
		slotModel[[2]int{s.DayNumber, s.SlotNumber}] = s
	}
	slotByAncode := make(map[int]*model.Slot)
	for _, pe := range planEntries {
		if !pe.InSlot() {
			continue // external, outside the period → no slot
		}
		if s := slotModel[[2]int{pe.DayNumber, pe.SlotNumber}]; s != nil {
			slotByAncode[pe.Ancode] = s
		}
	}
	return p.conflictsFromSlots(ctx, slotByAncode)
}

// conflictsFromSlots computes the aggregated conflicts for a given ancode->slot
// assignment (used both for the stored plan and for a freshly generated one).
func (p *Plexams) conflictsFromSlots(ctx context.Context, slotByAncode map[int]*model.Slot) ([]*model.ExamScheduleConflict, error) {
	students, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		return nil, err
	}

	type studInfo struct{ mtknr, name, program, group string }
	type agg struct {
		rank     int
		label    string
		students []studInfo
	}
	byPair := make(map[[2]int]*agg)
	for _, s := range students {
		placed := make([]int, 0, len(s.Regs))
		for _, ancode := range s.Regs {
			if _, ok := slotByAncode[ancode]; ok {
				placed = append(placed, ancode)
			}
		}
		sort.Ints(placed)
		for i := 0; i < len(placed); i++ {
			for j := i + 1; j < len(placed); j++ {
				rank, label := conflictcalc.SlotProximity(slotByAncode[placed[i]], slotByAncode[placed[j]])
				if rank <= 1 {
					continue // drop NEXT_DAY (and farther): always acceptable, handled by the objective
				}
				key := [2]int{placed[i], placed[j]}
				a := byPair[key]
				if a == nil {
					a = &agg{}
					byPair[key] = a
				}
				a.students = append(a.students, studInfo{s.Mtknr, s.Name, s.Program, s.Group})
				if rank > a.rank {
					a.rank, a.label = rank, label
				}
			}
		}
	}

	decisionByPair := make(map[[2]int]map[string]model.ConflictDecision) // pair -> mtknr -> decision
	if decs, err := p.dbClient.StudentConflictDecisions(ctx); err == nil {
		for _, d := range decs {
			key := [2]int{d.Ancode1, d.Ancode2}
			if decisionByPair[key] == nil {
				decisionByPair[key] = make(map[string]model.ConflictDecision)
			}
			decisionByPair[key][d.Mtknr] = d.Decision
		}
	}
	canShare := make(map[[2]int]bool)
	if pairs, err := p.dbClient.CanShareSlotPairs(ctx); err == nil {
		for _, pr := range pairs {
			canShare[[2]int{pr[0], pr[1]}] = true
		}
	}
	// sameSlot exams run simultaneously — a student can't sit both, so a conflict between
	// them is spurious: treat like canShareSlot (auto-accepted, info only).
	ssRoot := p.sameSlotGroups(ctx)
	info := p.examInfoMap(ctx)
	constraints, _ := p.ConstraintsMap(ctx)
	foreign := func(ancode int) bool {
		if ancode >= externalAncodeBase {
			return true
		}
		c := constraints[ancode]
		return c != nil && c.NotPlannedByMe
	}

	out := make([]*model.ExamScheduleConflict, 0, len(byPair))
	for key, a := range byPair {
		ep := examPair(key[0], key[1], info)
		i0, i1 := info[key[0]], info[key[1]]
		decs := decisionByPair[key]
		// canShareSlot declared, or the two are in the same sameSlot group -> no student
		// legitimately sits both.
		shareable := canShare[key]
		if r0, ok := ssRoot[key[0]]; ok && r0 == ssRoot[key[1]] {
			shareable = true
		}
		affected := make([]*model.ConflictStudent, 0, len(a.students))
		for _, s := range a.students {
			studSem := repeatcalc.SemesterOf(s.group)
			autoAccepted := shareable || repeatcalc.RepeatForStudent(studSem, i0.repeater, i0.minSem) || repeatcalc.RepeatForStudent(studSem, i1.repeater, i1.minSem)
			cs := &model.ConflictStudent{Mtknr: s.mtknr, Name: s.name, Program: s.program, Group: s.group, AutoAccepted: autoAccepted}
			if d, ok := decs[s.mtknr]; ok {
				dd := d
				cs.Decision = &dd
			}
			cs.Accepted = cs.Decision != nil && *cs.Decision == model.ConflictDecisionAccept ||
				(autoAccepted && (cs.Decision == nil || *cs.Decision != model.ConflictDecisionVeto))
			affected = append(affected, cs)
		}
		sort.Slice(affected, func(i, j int) bool { return affected[i].Name < affected[j].Name })
		c := &model.ExamScheduleConflict{
			Ancode1: ep.Ancode1, Module1: ep.Module1, MainExamer1: ep.MainExamer1, Groups1: i0.groups, IsRepeaterExam1: i0.repeater, Location1: locationOf(constraints[key[0]]), Slot1: slotByAncode[key[0]],
			Ancode2: ep.Ancode2, Module2: ep.Module2, MainExamer2: ep.MainExamer2, Groups2: i1.groups, IsRepeaterExam2: i1.repeater, Location2: locationOf(constraints[key[1]]), Slot2: slotByAncode[key[1]],
			StudentCount: len(a.students), Proximity: a.label, CanShareSlot: shareable,
			InfoOnly:         foreign(key[0]) && foreign(key[1]),
			AffectedStudents: affected,
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := conflictcalc.ProximityRank(out[i].Proximity), conflictcalc.ProximityRank(out[j].Proximity)
		if ri != rj {
			return ri > rj
		}
		if out[i].StudentCount != out[j].StudentCount {
			return out[i].StudentCount > out[j].StudentCount
		}
		return out[i].Ancode1 < out[j].Ancode1
	})
	return out, nil
}
