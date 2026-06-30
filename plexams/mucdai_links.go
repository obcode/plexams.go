package plexams

import (
	"context"
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
