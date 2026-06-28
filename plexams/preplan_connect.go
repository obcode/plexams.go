package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// ConnectPreplanExamToAncode links a pre-exam to a real ZPA exam by its ancode.
// The ancode must exist and must not already be linked by another pre-exam.
func (p *Plexams) ConnectPreplanExamToAncode(ctx context.Context, id, ancode int) (*model.PreplanExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	zpaExam, err := p.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		return nil, fmt.Errorf("no ZPA exam with ancode %d: %w", ancode, err)
	}
	if zpaExam == nil {
		return nil, fmt.Errorf("no ZPA exam with ancode %d", ancode)
	}

	// reject if another pre-exam already links this ancode
	all, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	for _, other := range all {
		if other.ID != id && other.Ancode != nil && *other.Ancode == ancode {
			return nil, fmt.Errorf("ancode %d is already linked by pre-exam %d (%s)", ancode, other.ID, other.Module)
		}
	}

	preExam.Ancode = &ancode
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preExam); err != nil {
		return nil, err
	}

	// carry the pre-planning constraints over to the real ZPA exam (same-slot
	// references are translated to the ancodes of the already-linked pre-exams).
	if preExam.Constraints != nil {
		input := p.preplanConstraintsToInput(ctx, preExam.Constraints)
		if _, err := p.AddConstraints(ctx, ancode, input); err != nil {
			return nil, fmt.Errorf("cannot carry over pre-plan constraints to ancode %d: %w", ancode, err)
		}
	}

	return preExam, nil
}

// DisconnectPreplanExam removes the ZPA link from a pre-exam.
func (p *Plexams) DisconnectPreplanExam(ctx context.Context, id int) (*model.PreplanExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}
	preExam.Ancode = nil
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preExam); err != nil {
		return nil, err
	}
	return preExam, nil
}

// PreplanExamAncodeSuggestions returns ZPA exams that are good candidates for
// linking the given pre-exam, ranked by examer (same teacher) and module-name
// similarity. Returns an empty list before the ZPA exams are imported.
func (p *Plexams) PreplanExamAncodeSuggestions(ctx context.Context, id int) ([]*model.ZPAExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	fromZpa := false
	zpaExams, err := p.GetZPAExams(ctx, &fromZpa)
	if err != nil {
		return nil, err
	}

	module := strings.ToLower(strings.TrimSpace(preExam.Module))

	type scored struct {
		exam  *model.ZPAExam
		score int // lower is better
	}
	candidates := make([]scored, 0)
	for _, ze := range zpaExams {
		sameExamer := ze.MainExamerID == preExam.ExamerID
		zeModule := strings.ToLower(strings.TrimSpace(ze.Module))

		moduleScore := 3
		switch {
		case zeModule == module && module != "":
			moduleScore = 0
		case module != "" && (strings.Contains(zeModule, module) || strings.Contains(module, zeModule)):
			moduleScore = 1
		}

		// keep only plausible candidates: same examer, or some module match
		if !sameExamer && moduleScore == 3 {
			continue
		}

		score := moduleScore
		if !sameExamer {
			score += 4 // examer match dominates
		}
		candidates = append(candidates, scored{exam: ze, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		return candidates[i].exam.AnCode < candidates[j].exam.AnCode
	})

	result := make([]*model.ZPAExam, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, c.exam)
	}
	return result, nil
}
