package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// MarkGeneratedExamsDirty flags the cached generated exams as stale (best-effort: a
// failure here never affects the triggering operation). Called whenever an input of
// the generation changes.
func (p *Plexams) MarkGeneratedExamsDirty(ctx context.Context, reason string) {
	if p.dbClient == nil {
		return
	}
	if err := p.dbClient.SetGeneratedExamsDirty(ctx, true, reason, time.Now()); err != nil {
		log.Error().Err(err).Str("reason", reason).Msg("cannot mark generated exams dirty")
	}
}

// GeneratedExamsState returns whether the cached generated exams are stale.
func (p *Plexams) GeneratedExamsState(ctx context.Context) (*model.GeneratedExamsState, error) {
	return p.dbClient.GetGeneratedExamsState(ctx)
}

// GenerateGeneratedExams regenerates the cached generated exams and returns the new
// (no longer dirty) state.
func (p *Plexams) GenerateGeneratedExams(ctx context.Context) (*model.GeneratedExamsState, error) {
	if err := p.PrepareGeneratedExams(); err != nil {
		log.Error().Err(err).Msg("cannot regenerate generated exams")
		return nil, err
	}
	return p.GeneratedExamsState(ctx)
}
