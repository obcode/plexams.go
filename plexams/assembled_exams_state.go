package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// MarkAssembledExamsDirty flags the cached assembled exams as stale (best-effort: a
// failure here never affects the triggering operation). Called whenever an input of
// the generation changes.
func (p *Plexams) MarkAssembledExamsDirty(ctx context.Context, reason string) {
	if p.dbClient == nil {
		return
	}
	// nothing can be "stale" before the exams have been generated at least once (e.g.
	// the first ZPA import, before any Primuss data, must not raise the banner).
	if n, err := p.dbClient.CountAssembledExams(ctx); err == nil && n == 0 {
		return
	}
	if err := p.dbClient.SetAssembledExamsDirty(ctx, true, reason, time.Now()); err != nil {
		log.Error().Err(err).Str("reason", reason).Msg("cannot mark assembled exams dirty")
	}
}

// AssembledExamsState returns whether the cached assembled exams are stale. They can
// only be stale once they have been generated at least once; before that (e.g. right
// after the first ZPA import) the state is reported as not dirty.
func (p *Plexams) AssembledExamsState(ctx context.Context) (*model.AssembledExamsState, error) {
	state, err := p.dbClient.GetAssembledExamsState(ctx)
	if err != nil {
		return nil, err
	}
	if state != nil && state.Dirty {
		if n, err := p.dbClient.CountAssembledExams(ctx); err == nil && n == 0 {
			state.Dirty = false
		}
	}
	return state, nil
}

// ResetAssembledExams deletes the cached assembled exams and their state, undoing a
// generation; they can be rebuilt with GenerateAssembledExams. Returns how many
// assembled exams were removed. Blocked while a validation or transfer/email is
// running.
func (p *Plexams) ResetAssembledExams(ctx context.Context) (int, error) {
	if !p.WritesAllowed() {
		return 0, fmt.Errorf("a validation or transfer/email is running, cannot reset now")
	}
	n, err := p.dbClient.DropAssembledExams(ctx)
	if err != nil {
		return 0, err
	}
	p.unmarkCondition(ctx, condAssembledExams)
	return int(n), nil
}

// GenerateAssembledExams regenerates the cached assembled exams and returns the new
// (no longer dirty) state together with the changes vs the previous cache.
func (p *Plexams) GenerateAssembledExams(ctx context.Context) (*model.GenerateAssembledExamsResult, error) {
	// snapshot the previous cache before it is overwritten
	old, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("cannot read previous assembled exams (treating as empty)")
		old = nil
	}

	if err := p.PrepareAssembledExams(); err != nil {
		log.Error().Err(err).Msg("cannot regenerate assembled exams")
		return nil, err
	}

	newExams, err := p.dbClient.GetAssembledExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read reassembled exams")
		return nil, err
	}

	state, err := p.AssembledExamsState(ctx)
	if err != nil {
		return nil, err
	}

	return &model.GenerateAssembledExamsResult{
		State:   state,
		Changes: diffAssembledExams(old, newExams),
	}, nil
}

// diffAssembledExams compares two sets of assembled exams by ancode and reports the
// added, removed and changed exams (changed: a difference in the relevant derived
// numbers).
func diffAssembledExams(old, newExams []*model.AssembledExam) []*model.AssembledExamsChange {
	oldMap := make(map[int]*model.AssembledExam, len(old))
	for _, e := range old {
		oldMap[e.Ancode] = e
	}
	newMap := make(map[int]*model.AssembledExam, len(newExams))
	for _, e := range newExams {
		newMap[e.Ancode] = e
	}

	changes := make([]*model.AssembledExamsChange, 0)

	for _, e := range newExams {
		o, ok := oldMap[e.Ancode]
		if !ok {
			changes = append(changes, &model.AssembledExamsChange{
				Ancode: e.Ancode, Module: e.ZpaExam.Module, Kind: "added",
				Details: []string{fmt.Sprintf("neu (%d Anmeldungen)", e.StudentRegsCount)},
			})
			continue
		}
		if details := assembledExamDiffDetails(o, e); len(details) > 0 {
			changes = append(changes, &model.AssembledExamsChange{
				Ancode: e.Ancode, Module: e.ZpaExam.Module, Kind: "changed", Details: details,
			})
		}
	}

	for _, o := range old {
		if _, ok := newMap[o.Ancode]; !ok {
			changes = append(changes, &model.AssembledExamsChange{
				Ancode: o.Ancode, Module: o.ZpaExam.Module, Kind: "removed",
				Details: []string{"nicht mehr vorhanden"},
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Ancode < changes[j].Ancode })
	return changes
}

// assembledExamDiffDetails lists the differences in the relevant derived numbers of
// two assembled exams with the same ancode.
func assembledExamDiffDetails(o, n *model.AssembledExam) []string {
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
