package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// mucdaiProgramNames returns the MUC.DAI program shortnames from the StudyProgram
// master data (category mucdai).
func (p *Plexams) mucdaiProgramNames(ctx context.Context) []string {
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read study programs for mucdai")
	}
	names := make([]string, 0)
	for _, prog := range programs {
		if prog.Category == "mucdai" {
			names = append(names, prog.Shortname)
		}
	}
	return names
}

func (p *Plexams) MucdaiExams(ctx context.Context) ([]*model.MucDaiExam, error) {
	mucdaiPrograms := p.mucdaiProgramNames(ctx)

	exams := make([]*model.MucDaiExam, 0)

	for _, program := range mucdaiPrograms {
		examsForProgram, err := p.MucDaiExamsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get mucdai exams for program")
		} else {
			exams = append(exams, examsForProgram...)
		}
	}

	p.enrichMucDaiExams(ctx, exams)
	return exams, nil
}

// enrichMucDaiExams fills each MUC.DAI exam's link status, linked ancode and plan entry
// from the explicit mucdai_links collection (built at import time / via manual links).
func (p *Plexams) enrichMucDaiExams(ctx context.Context, exams []*model.MucDaiExam) {
	links, err := p.dbClient.MucDaiLinks(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get mucdai links for enrichment")
		return
	}
	linkByKey := make(map[primussKey]*db.MucDaiLink, len(links))
	for _, l := range links {
		linkByKey[primussKey{l.Program, l.PrimussAncode}] = l
	}

	planEntries := make(map[int]*model.PlanEntry)
	if entries, err := p.dbClient.PlanEntries(ctx); err != nil {
		log.Error().Err(err).Msg("cannot get plan entries for mucdai enrichment")
	} else {
		for _, entry := range entries {
			planEntries[entry.Ancode] = entry
		}
	}

	for _, exam := range exams {
		link := linkByKey[primussKey{exam.Program, exam.PrimussAncode}]
		if link == nil || link.Status == "unresolved" {
			exam.LinkStatus = mucDaiLinkUnresolved
			continue
		}
		exam.LinkStatus = link.Kind // "external" | "zpa"
		if link.Ancode != nil {
			a := *link.Ancode
			exam.Ancode = &a
			if entry, ok := planEntries[a]; ok {
				exam.PlanEntry = entry
			}
		}
	}
}

func (p *Plexams) MucDaiExamsForProgram(ctx context.Context, program string) ([]*model.MucDaiExam, error) {
	exams, err := p.dbClient.MucDaiExamsForProgram(ctx, program)
	if err != nil {
		return nil, err
	}
	mucdaiExams := make([]*model.MucDaiExam, 0, len(exams))
	for _, exam := range exams {
		mucdaiExams = append(mucdaiExams, p.mkMucdaiExam(exam))
	}

	return mucdaiExams, nil
}

func (p *Plexams) MucDaiExam(ctx context.Context, program string, ancode int) (*model.MucDaiExam, error) {
	exam, err := p.dbClient.MucDaiExam(ctx, program, ancode)
	if err != nil {
		return nil, err
	}
	return p.mkMucdaiExam(exam), nil
}

func (p *Plexams) mkMucdaiExam(mucdaiExam *db.MucDaiExam) *model.MucDaiExam {
	isRepeaterExam := mucdaiExam.IsRepeaterExam == "x"

	return &model.MucDaiExam{
		PrimussAncode:  mucdaiExam.PrimussAncode,
		Module:         mucdaiExam.Module,
		MainExamer:     mucdaiExam.MainExamer,
		ExamType:       mucdaiExam.ExamType,
		Duration:       mucdaiExam.Duration,
		IsRepeaterExam: isRepeaterExam,
		Program:        mucdaiExam.Program,
		PlannedBy:      mucdaiExam.Planer,
	}
}

// AddMucDaiExamByProgram adds a MUC.DAI exam, deriving the local ZPA ancode from the
// program's externalExamsBase (base + primussAncode) in the StudyProgram master data.
func (p *Plexams) AddMucDaiExamByProgram(ctx context.Context, mucdaiExam *model.MucDaiExam) (*model.ZPAExam, error) {
	base, ok := p.externalExamsBaseForProgram(ctx, mucdaiExam.Program)
	if !ok {
		return nil, fmt.Errorf("no externalExamsBase set for program %s (StudyProgram master data)", mucdaiExam.Program)
	}
	return p.AddMucDaiExam(ctx, base+mucdaiExam.PrimussAncode, mucdaiExam)
}

func (p *Plexams) AddMucDaiExam(ctx context.Context, zpaAncode int, mucdaiExam *model.MucDaiExam) (*model.ZPAExam, error) {
	zpaExam := &model.ZPAExam{
		ZpaID:          0,
		Semester:       p.semester,
		AnCode:         zpaAncode,
		Module:         mucdaiExam.Module,
		MainExamer:     mucdaiExam.MainExamer,
		MainExamerID:   0,
		ExamType:       mucdaiExam.ExamType,
		ExamTypeFull:   "",
		Date:           "",
		Starttime:      "",
		Duration:       mucdaiExam.Duration,
		IsRepeaterExam: mucdaiExam.IsRepeaterExam,
		Groups:         []string{},
		PrimussAncodes: []model.ZPAPrimussAncodes{{
			Program: mucdaiExam.Program,
			Ancode:  mucdaiExam.PrimussAncode,
		}},
	}

	err := p.dbClient.AddNonZpaExam(ctx, zpaExam)

	return zpaExam, err
}
