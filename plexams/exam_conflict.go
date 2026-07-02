package plexams

import (
	"context"
	"fmt"
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
