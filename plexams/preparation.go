package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// GeneratePreparation regenerates both the cached assembled exams and the per-student
// planned registrations in one step. They share the same inputs (connected exams +
// Primuss data) and do not depend on each other's output, so they are always
// (re)generated together. Each sub-step clears its own stale flag and marks its
// planning-state condition.
func (p *Plexams) GeneratePreparation(ctx context.Context) (*model.GeneratePreparationResult, error) {
	assembledExams, err := p.GenerateAssembledExams(ctx)
	if err != nil {
		return nil, err
	}
	studentRegs, err := p.GenerateStudentRegs(ctx)
	if err != nil {
		return nil, err
	}
	return &model.GeneratePreparationResult{
		AssembledExams: assembledExams,
		StudentRegs:    studentRegs,
	}, nil
}
