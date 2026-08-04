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
	updated.Deactivated = existing.Deactivated
	updated.LastSemester = existing.LastSemester

	return p.dbClient.ReplaceNta(ctx, updated)
}

// SetNTAActive activates/deactivates an NTA (key: mtknr). A deactivated NTA is
// not added to exams during prepare/generate. Errors if the mtknr does not exist.
func (p *Plexams) SetNTAActive(ctx context.Context, mtknr string, active bool) (*model.NTA, error) {
	nta, err := p.dbClient.SetNtaDeactivated(ctx, mtknr, !active)
	if err != nil {
		return nil, err
	}
	if nta == nil {
		return nil, fmt.Errorf("no nta with mtknr %s", mtknr)
	}
	return nta, nil
}

func (p *Plexams) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return p.dbClient.Ntas(ctx)
}

func (p *Plexams) NtasWithRegs(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.NtasWithRegs(ctx)
}

// Nta always returns (nil, nil) -- there is nothing left to read.
//
// It read the per-semester `nta` collection, which nothing has written since
// 2023-SS: the prepared student view (studentregs_per_student_planned) replaced
// it, the producer was removed and this reader stayed. So it has answered "not
// found" for about three years, and the GUI page /nta/<mtknr> -- linked from
// /nta/all for every mtknr -- has shown "Kein NTA mit dieser Matrikelnummer
// gefunden" for just as long.
//
// Kept, rather than deleted, so the flip changes no behaviour and no GraphQL
// field. Fixing it belongs in the GUI: point the page at StudentByMtknr (which
// is ported), then this, its resolver and its query can go.
//
// Deprecated: use StudentByMtknr
func (p *Plexams) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return nil, nil
}

func (p *Plexams) NtaByMtknr(ctx context.Context, mtknr string) (*model.NTA, error) {
	return p.dbClient.Nta(ctx, mtknr)
}

func (p *Plexams) ExamsWithNtas(ctx context.Context) ([]*model.PlannedExam, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
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
