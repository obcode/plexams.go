package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// MUC.DAI link status values exposed on model.MucDaiExam.LinkStatus.
const (
	mucDaiLinkExternal   = "external"   // auto-created external exam, linked
	mucDaiLinkZPA        = "zpa"        // linked to a ZPA exam
	mucDaiLinkUnresolved = "unresolved" // FK07 exam without a clear ZPA match
)

// relinkMucDaiExams (re)builds the explicit mucdai_links collection for all imported
// MUC.DAI exams: non-FK07 exams link to their auto-created external exam, FK07 exams to
// the ZPA exam whose primussAncodes contain (program, primussAncode) exactly (unique
// match → linked, otherwise unresolved). Manual links (source=manual) are preserved.
// Links for exams no longer present are removed.
func (p *Plexams) relinkMucDaiExams(ctx context.Context) error {
	programs := p.mucdaiProgramNames(ctx)

	existing, err := p.dbClient.MucDaiLinks(ctx)
	if err != nil {
		return err
	}
	linkByKey := make(map[primussKey]*db.MucDaiLink, len(existing))
	for _, l := range existing {
		linkByKey[primussKey{l.Program, l.PrimussAncode}] = l
	}

	nonZpaMap, _, err := p.existingNonZpaByPrimuss(ctx)
	if err != nil {
		return err
	}

	zpaByPrimuss, err := p.zpaExamsByPrimussAncode(ctx)
	if err != nil {
		return err
	}

	valid := make(map[primussKey]bool)
	for _, program := range programs {
		exams, err := p.MucDaiExamsForProgram(ctx, program)
		if err != nil {
			return err
		}
		for _, e := range exams {
			key := primussKey{e.Program, e.PrimussAncode}
			valid[key] = true

			// preserve a manual link, only refreshing the display snapshot
			if l := linkByKey[key]; l != nil && l.Source == "manual" {
				l.Module, l.MainExamer = e.Module, e.MainExamer
				if err := p.dbClient.UpsertMucDaiLink(ctx, l); err != nil {
					return err
				}
				continue
			}

			if err := p.dbClient.UpsertMucDaiLink(ctx, autoMucDaiLink(e, zpaByPrimuss, nonZpaMap)); err != nil {
				return err
			}
		}
	}

	// drop links of exams that are no longer in the imported data
	for key := range linkByKey {
		if !valid[key] {
			if err := p.dbClient.DeleteMucDaiLink(ctx, key.program, key.ancode); err != nil {
				log.Error().Err(err).Str("program", key.program).Int("ancode", key.ancode).
					Msg("cannot delete stale mucdai link")
			}
		}
	}
	return nil
}

// autoMucDaiLink computes the automatic link for one MUC.DAI exam: FK07-planned exams
// link to the ZPA exam whose primussAncodes contain (program, primussAncode) exactly
// (unique match → linked, else unresolved); other faculties' exams link to their
// auto-created external exam. Pure (no DB), so it is unit-testable.
func autoMucDaiLink(e *model.MucDaiExam, zpaByPrimuss map[primussKey][]int, nonZpaMap map[primussKey]int) *db.MucDaiLink {
	key := primussKey{e.Program, e.PrimussAncode}
	link := &db.MucDaiLink{
		Program: e.Program, PrimussAncode: e.PrimussAncode,
		Source: "auto", Module: e.Module, MainExamer: e.MainExamer,
		Status: "unresolved",
	}
	if strings.EqualFold(strings.TrimSpace(e.PlannedBy), mucDaiPlannerFK07) {
		link.Kind = mucDaiLinkZPA
		if zas := zpaByPrimuss[key]; len(zas) == 1 {
			a := zas[0]
			link.Ancode, link.Status = &a, "linked"
		}
	} else {
		link.Kind = mucDaiLinkExternal
		if a, ok := nonZpaMap[key]; ok {
			aa := a
			link.Ancode, link.Status = &aa, "linked"
		}
	}
	return link
}

// SetMucDaiZpaLink manually links a MUC.DAI exam to a ZPA exam (the unresolved/wrong
// FK07 cases). Stored as a manual link that survives re-imports. Returns the updated exam.
func (p *Plexams) SetMucDaiZpaLink(ctx context.Context, program string, primussAncode, zpaAncode int) (*model.MucDaiExam, error) {
	mucExam, err := p.dbClient.MucDaiExam(ctx, program, primussAncode)
	if err != nil || mucExam == nil {
		return nil, fmt.Errorf("no MUC.DAI exam %s/%d", program, primussAncode)
	}
	zpaExam, err := p.GetZpaExamByAncode(ctx, zpaAncode)
	if err != nil || zpaExam == nil {
		return nil, fmt.Errorf("no ZPA exam with ancode %d", zpaAncode)
	}
	a := zpaAncode
	if err := p.dbClient.UpsertMucDaiLink(ctx, &db.MucDaiLink{
		Program: program, PrimussAncode: primussAncode,
		Kind: mucDaiLinkZPA, Ancode: &a, Status: "linked", Source: "manual",
		Module: mucExam.Module, MainExamer: mucExam.MainExamer,
	}); err != nil {
		return nil, err
	}
	return p.enrichedMucDaiExam(ctx, program, primussAncode)
}

// RemoveMucDaiLink drops a (manual) link and falls back to automatic detection.
func (p *Plexams) RemoveMucDaiLink(ctx context.Context, program string, primussAncode int) (*model.MucDaiExam, error) {
	mucExam, err := p.dbClient.MucDaiExam(ctx, program, primussAncode)
	if err != nil || mucExam == nil {
		return nil, fmt.Errorf("no MUC.DAI exam %s/%d", program, primussAncode)
	}
	nonZpaMap, _, err := p.existingNonZpaByPrimuss(ctx)
	if err != nil {
		return nil, err
	}
	zpaByPrimuss, err := p.zpaExamsByPrimussAncode(ctx)
	if err != nil {
		return nil, err
	}
	if err := p.dbClient.UpsertMucDaiLink(ctx, autoMucDaiLink(p.mkMucdaiExam(mucExam), zpaByPrimuss, nonZpaMap)); err != nil {
		return nil, err
	}
	return p.enrichedMucDaiExam(ctx, program, primussAncode)
}

// enrichedMucDaiExam loads one MUC.DAI exam and fills its link status/ancode/plan entry.
func (p *Plexams) enrichedMucDaiExam(ctx context.Context, program string, primussAncode int) (*model.MucDaiExam, error) {
	mucExam, err := p.dbClient.MucDaiExam(ctx, program, primussAncode)
	if err != nil || mucExam == nil {
		return nil, fmt.Errorf("no MUC.DAI exam %s/%d", program, primussAncode)
	}
	exam := p.mkMucdaiExam(mucExam)
	p.enrichMucDaiExams(ctx, []*model.MucDaiExam{exam})
	return exam, nil
}

// MucDaiZpaCandidates suggests ZPA exams for linking an (unresolved) MUC.DAI exam,
// ranked: the program carried with a missing number (0/-1) first, then same examer +
// similar module, then either.
func (p *Plexams) MucDaiZpaCandidates(ctx context.Context, program string, primussAncode int) ([]*model.ZPAExam, error) {
	muc, err := p.dbClient.MucDaiExam(ctx, program, primussAncode)
	if err != nil {
		return nil, err
	}
	fromZPA := false
	zpaExams, err := p.GetZPAExams(ctx, &fromZPA)
	if err != nil {
		return nil, err
	}

	type scored struct {
		exam  *model.ZPAExam
		score int
	}
	cands := make([]scored, 0)
	for _, ze := range zpaExams {
		score := -1
		for _, pa := range ze.PrimussAncodes {
			if pa.Program != program {
				continue
			}
			if pa.Ancode == primussAncode {
				score = 0 // exact (e.g. correcting an existing link)
			} else if pa.Ancode <= 0 {
				score = best(score, 1) // program present with a missing number
			}
		}
		if muc != nil {
			sameEx := sameExamer(muc.MainExamer, ze.MainExamer)
			simMod := similarModule(muc.Module, ze.Module)
			switch {
			case sameEx && simMod:
				score = best(score, 2)
			case sameEx:
				score = best(score, 3)
			case simMod:
				score = best(score, 4)
			}
		}
		if score >= 0 {
			cands = append(cands, scored{ze, score})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score < cands[j].score
		}
		return cands[i].exam.AnCode < cands[j].exam.AnCode
	})

	result := make([]*model.ZPAExam, 0, len(cands))
	for _, c := range cands {
		result = append(result, c.exam)
	}
	return result, nil
}

// best returns the higher-priority (smaller, but >= 0) of two scores.
func best(cur, candidate int) int {
	if cur < 0 || candidate < cur {
		return candidate
	}
	return cur
}

// zpaExamsByPrimussAncode maps (program, primussAncode) to the ZPA ancodes whose
// primussAncodes contain it (only real, positive primuss ancodes).
func (p *Plexams) zpaExamsByPrimussAncode(ctx context.Context) (map[primussKey][]int, error) {
	fromZPA := false
	zpaExams, err := p.GetZPAExams(ctx, &fromZPA)
	if err != nil {
		return nil, err
	}
	m := make(map[primussKey][]int)
	for _, ze := range zpaExams {
		for _, pa := range ze.PrimussAncodes {
			if pa.Ancode > 0 {
				key := primussKey{pa.Program, pa.Ancode}
				m[key] = append(m[key], ze.AnCode)
			}
		}
	}
	return m, nil
}
