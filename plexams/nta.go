package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AddNta(ctx context.Context, input model.NTAInput) (*model.NTA, error) {
	existing, err := p.dbClient.Nta(ctx, input.Mtknr)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("nta with mtknr %s already exists", input.Mtknr)
	}
	return p.dbClient.AddNta(ctx, model.NtaInputToNta(input))
}

// UpdateNta updates the editable fields of an existing NTA (identified by its
// mtknr), preserving the server-managed fields exams, deactivated and
// lastSemester. It errors if no NTA with that mtknr exists.
func (p *Plexams) UpdateNta(ctx context.Context, input model.NTAInput) (*model.NTA, error) {
	existing, err := p.dbClient.Nta(ctx, input.Mtknr)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("no nta with mtknr %s", input.Mtknr)
	}

	updated := model.NtaInputToNta(input)
	// keep server-managed fields
	updated.Exams = existing.Exams
	updated.Deactivated = existing.Deactivated
	updated.LastSemester = existing.LastSemester

	return p.dbClient.ReplaceNta(ctx, updated)
}

func (p *Plexams) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return p.dbClient.Ntas(ctx)
}

func (p *Plexams) NtasWithRegs(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.NtasWithRegs(ctx)
}

// Deprecated: use StudentByMtknr
func (p *Plexams) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return p.dbClient.NtaWithRegs(ctx, mtknr)
}

func (p *Plexams) NtaByMtknr(ctx context.Context, mtknr string) (*model.NTA, error) {
	return p.dbClient.Nta(ctx, mtknr)
}

func (p *Plexams) ExamsWithNtas(ctx context.Context) ([]*model.PlannedExam, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
	}
	exams := make([]*model.PlannedExam, 0)
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		if len(exam.Ntas) == 0 {
			continue
		}
		exams = append(exams, exam)
	}
	return exams, nil
}
