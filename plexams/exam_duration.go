package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// SetExamDuration sets the duration override (minutes) for an ancode. It is only
// applied to assembled exams whose ZPA duration is 0.
func (p *Plexams) SetExamDuration(ctx context.Context, ancode, duration int) (*model.ExamDurationOverride, error) {
	return p.dbClient.SetExamDurationOverride(ctx, ancode, duration)
}

// RemoveExamDuration removes the duration override for an ancode.
func (p *Plexams) RemoveExamDuration(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.RemoveExamDurationOverride(ctx, ancode)
}

// examDurationOverridesMap returns the duration overrides as an ancode->minutes map.
func (p *Plexams) examDurationOverridesMap(ctx context.Context) (map[int]int, error) {
	overrides, err := p.dbClient.ExamDurationOverrides(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[int]int, len(overrides))
	for _, o := range overrides {
		m[o.Ancode] = o.Duration
	}
	return m, nil
}
