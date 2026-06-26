package plexams

import (
	"context"
	"fmt"
	"sort"
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
// (no longer dirty) state together with the changes vs the previous cache.
func (p *Plexams) GenerateGeneratedExams(ctx context.Context) (*model.GenerateGeneratedExamsResult, error) {
	// snapshot the previous cache before it is overwritten
	old, err := p.dbClient.GetGeneratedExams(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("cannot read previous generated exams (treating as empty)")
		old = nil
	}

	if err := p.PrepareGeneratedExams(); err != nil {
		log.Error().Err(err).Msg("cannot regenerate generated exams")
		return nil, err
	}

	newExams, err := p.dbClient.GetGeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read regenerated exams")
		return nil, err
	}

	state, err := p.GeneratedExamsState(ctx)
	if err != nil {
		return nil, err
	}

	return &model.GenerateGeneratedExamsResult{
		State:   state,
		Changes: diffGeneratedExams(old, newExams),
	}, nil
}

// diffGeneratedExams compares two sets of generated exams by ancode and reports the
// added, removed and changed exams (changed: a difference in the relevant derived
// numbers).
func diffGeneratedExams(old, newExams []*model.GeneratedExam) []*model.GeneratedExamsChange {
	oldMap := make(map[int]*model.GeneratedExam, len(old))
	for _, e := range old {
		oldMap[e.Ancode] = e
	}
	newMap := make(map[int]*model.GeneratedExam, len(newExams))
	for _, e := range newExams {
		newMap[e.Ancode] = e
	}

	changes := make([]*model.GeneratedExamsChange, 0)

	for _, e := range newExams {
		o, ok := oldMap[e.Ancode]
		if !ok {
			changes = append(changes, &model.GeneratedExamsChange{
				Ancode: e.Ancode, Module: e.ZpaExam.Module, Kind: "added",
				Details: []string{fmt.Sprintf("neu (%d Anmeldungen)", e.StudentRegsCount)},
			})
			continue
		}
		if details := generatedExamDiffDetails(o, e); len(details) > 0 {
			changes = append(changes, &model.GeneratedExamsChange{
				Ancode: e.Ancode, Module: e.ZpaExam.Module, Kind: "changed", Details: details,
			})
		}
	}

	for _, o := range old {
		if _, ok := newMap[o.Ancode]; !ok {
			changes = append(changes, &model.GeneratedExamsChange{
				Ancode: o.Ancode, Module: o.ZpaExam.Module, Kind: "removed",
				Details: []string{"nicht mehr vorhanden"},
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Ancode < changes[j].Ancode })
	return changes
}

// generatedExamDiffDetails lists the differences in the relevant derived numbers of
// two generated exams with the same ancode.
func generatedExamDiffDetails(o, n *model.GeneratedExam) []string {
	details := make([]string, 0)
	add := func(label string, a, b int) {
		if a != b {
			details = append(details, fmt.Sprintf("%s %d → %d", label, a, b))
		}
	}
	add("Anmeldungen", o.StudentRegsCount, n.StudentRegsCount)
	add("Konflikte", len(o.Conflicts), len(n.Conflicts))
	add("NTAs", len(o.Ntas), len(n.Ntas))
	add("Max. Dauer", o.MaxDuration, n.MaxDuration)
	add("Primuss-Prüfungen", len(o.PrimussExams), len(n.PrimussExams))
	return details
}
