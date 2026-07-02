package plexams

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// normPair returns the two ancodes in ascending order (ratings/canShareSlot are stored
// order-independently).
func normPair(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}

// ConflictRatings returns all stored conflict ratings.
func (p *Plexams) ConflictRatings(ctx context.Context) ([]*model.ExamConflictRating, error) {
	return p.dbClient.ConflictRatings(ctx)
}

// SetConflictRating stores a conflict rating. ACCEPTED requires an mtknr (per-student);
// UNDESIRED/FORBIDDEN must not have one (pair-level).
func (p *Plexams) SetConflictRating(ctx context.Context, ancode1, ancode2 int, rating model.ConflictRating, mtknr *string) (bool, error) {
	if !rating.IsValid() {
		return false, fmt.Errorf("invalid rating %q", rating)
	}
	if ancode1 == ancode2 {
		return false, fmt.Errorf("cannot rate an exam against itself")
	}
	if rating == model.ConflictRatingAccepted && (mtknr == nil || *mtknr == "") {
		return false, fmt.Errorf("ACCEPTED requires an mtknr (per-student)")
	}
	if rating != model.ConflictRatingAccepted && mtknr != nil && *mtknr != "" {
		return false, fmt.Errorf("%s is pair-level and must not have an mtknr", rating)
	}
	a, b := normPair(ancode1, ancode2)
	m := ""
	if mtknr != nil {
		m = *mtknr
	}
	if err := p.dbClient.UpsertConflictRating(ctx, a, b, string(rating), m); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveConflictRating deletes a conflict rating (pass mtknr for a per-student one).
func (p *Plexams) RemoveConflictRating(ctx context.Context, ancode1, ancode2 int, mtknr *string) (bool, error) {
	a, b := normPair(ancode1, ancode2)
	m := ""
	if mtknr != nil {
		m = *mtknr
	}
	return p.dbClient.DeleteConflictRating(ctx, a, b, m)
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
	a, b := normPair(ancode1, ancode2)
	if err := p.dbClient.UpsertCanShareSlot(ctx, a, b); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveExamsCanShareSlot undeclares a can-share-slot pair.
func (p *Plexams) RemoveExamsCanShareSlot(ctx context.Context, ancode1, ancode2 int) (bool, error) {
	a, b := normPair(ancode1, ancode2)
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
				a, b := normPair(list[i].Ancode, list[j].Ancode)
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
	module string
	examer string
}

func (p *Plexams) examInfoMap(ctx context.Context) map[int]examInfo {
	m := make(map[int]examInfo)
	assembled, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		return m
	}
	for _, e := range assembled {
		m[e.Ancode] = examInfo{module: e.ZpaExam.Module, examer: e.ZpaExam.MainExamer}
	}
	return m
}

func examPair(a, b int, info map[int]examInfo) *model.ExamPair {
	return &model.ExamPair{
		Ancode1: a, Module1: info[a].module, MainExamer1: info[a].examer,
		Ancode2: b, Module2: info[b].module, MainExamer2: info[b].examer,
	}
}

// proximity rank/labels of two placed slots (higher = closer/worse); 0 = far enough
// to not count as a conflict.
func slotProximity(a, b *model.Slot) (int, string) {
	if a.DayNumber == b.DayNumber {
		switch absInt(a.SlotNumber - b.SlotNumber) {
		case 0:
			return 4, "SAME_SLOT"
		case 1:
			return 3, "ADJACENT"
		default:
			return 2, "SAME_DAY"
		}
	}
	diff := a.Starttime.Sub(b.Starttime)
	if diff < 0 {
		diff = -diff
	}
	if int(math.Round(diff.Hours()/24)) == 1 {
		return 1, "NEXT_DAY"
	}
	return 0, ""
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
		if pe.DayNumber == 0 && pe.SlotNumber == 0 {
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

	type studInfo struct{ mtknr, name string }
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
				rank, label := slotProximity(slotByAncode[placed[i]], slotByAncode[placed[j]])
				if rank == 0 {
					continue
				}
				key := [2]int{placed[i], placed[j]}
				a := byPair[key]
				if a == nil {
					a = &agg{}
					byPair[key] = a
				}
				a.students = append(a.students, studInfo{s.Mtknr, s.Name})
				if rank > a.rank {
					a.rank, a.label = rank, label
				}
			}
		}
	}

	ratingByPair := make(map[[2]int]model.ConflictRating)
	acceptedByPair := make(map[[2]int]map[string]bool) // pair -> mtknr set (ACCEPTED)
	if ratings, err := p.dbClient.ConflictRatings(ctx); err == nil {
		for _, r := range ratings {
			key := [2]int{r.Ancode1, r.Ancode2}
			if r.Mtknr == nil {
				ratingByPair[key] = r.Rating // pair-level
			} else if r.Rating == model.ConflictRatingAccepted {
				if acceptedByPair[key] == nil {
					acceptedByPair[key] = make(map[string]bool)
				}
				acceptedByPair[key][*r.Mtknr] = true
			}
		}
	}
	canShare := make(map[[2]int]bool)
	if pairs, err := p.dbClient.CanShareSlotPairs(ctx); err == nil {
		for _, pr := range pairs {
			canShare[[2]int{pr[0], pr[1]}] = true
		}
	}
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
		acc := acceptedByPair[key]
		affected := make([]*model.ConflictStudent, 0, len(a.students))
		for _, s := range a.students {
			affected = append(affected, &model.ConflictStudent{Mtknr: s.mtknr, Name: s.name, Accepted: acc[s.mtknr]})
		}
		sort.Slice(affected, func(i, j int) bool { return affected[i].Name < affected[j].Name })
		c := &model.ExamScheduleConflict{
			Ancode1: ep.Ancode1, Module1: ep.Module1, MainExamer1: ep.MainExamer1,
			Ancode2: ep.Ancode2, Module2: ep.Module2, MainExamer2: ep.MainExamer2,
			StudentCount: len(a.students), Proximity: a.label, CanShareSlot: canShare[key],
			InfoOnly:         foreign(key[0]) && foreign(key[1]),
			AffectedStudents: affected,
		}
		if r, ok := ratingByPair[key]; ok {
			c.Rating = &r
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := proxRank(out[i].Proximity), proxRank(out[j].Proximity)
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

func proxRank(label string) int {
	switch label {
	case "SAME_SLOT":
		return 4
	case "ADJACENT":
		return 3
	case "SAME_DAY":
		return 2
	case "NEXT_DAY":
		return 1
	}
	return 0
}
