package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// MarkStudentRegsDirty flags the prepared student regs as stale (best-effort).
func (p *Plexams) MarkStudentRegsDirty(ctx context.Context, reason string) {
	if p.dbClient == nil {
		return
	}
	if err := p.dbClient.SetStudentRegsDirty(ctx, true, reason, time.Now()); err != nil {
		log.Error().Err(err).Str("reason", reason).Msg("cannot mark student regs dirty")
	}
}

// StudentRegsState returns whether the prepared student regs are stale.
func (p *Plexams) StudentRegsState(ctx context.Context) (*model.StudentRegsState, error) {
	return p.dbClient.GetStudentRegsState(ctx)
}

// GenerateStudentRegs regenerates the per-student planned registrations and returns
// the new (no longer dirty) state together with the number of students.
func (p *Plexams) GenerateStudentRegs(ctx context.Context) (*model.GenerateStudentRegsResult, error) {
	if err := p.PrepareStudentRegs(); err != nil {
		log.Error().Err(err).Msg("cannot regenerate student regs")
		return nil, err
	}

	state, err := p.StudentRegsState(ctx)
	if err != nil {
		return nil, err
	}

	students, err := p.dbClient.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		return nil, err
	}

	return &model.GenerateStudentRegsResult{
		State:        state,
		StudentCount: len(students),
	}, nil
}
