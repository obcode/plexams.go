package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// jointProgramNames returns the MUC.DAI program shortnames from the StudyProgram
// master data (category joint).
func (p *Plexams) jointProgramNames(ctx context.Context) []string {
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read study programs for joint")
	}
	names := make([]string, 0)
	for _, prog := range programs {
		if prog.Category == "joint" {
			names = append(names, prog.Shortname)
		}
	}
	return names
}

func (p *Plexams) JointExams(ctx context.Context) ([]*model.JointExam, error) {
	jointPrograms := p.jointProgramNames(ctx)

	exams := make([]*model.JointExam, 0)

	for _, program := range jointPrograms {
		examsForProgram, err := p.JointExamsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get joint exams for program")
		} else {
			exams = append(exams, examsForProgram...)
		}
	}

	p.enrichJointExams(ctx, exams)
	return exams, nil
}

// enrichJointExams fills each MUC.DAI exam's link status, linked ancode and plan entry
// from the explicit joint_links collection (built at import time / via manual links).
func (p *Plexams) enrichJointExams(ctx context.Context, exams []*model.JointExam) {
	links, err := p.dbClient.JointLinks(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get joint links for enrichment")
		return
	}
	linkByKey := make(map[primussKey]*db.JointLink, len(links))
	for _, l := range links {
		linkByKey[primussKey{l.Program, l.PrimussAncode}] = l
	}

	planEntries := make(map[int]*model.PlanEntry)
	if entries, err := p.dbClient.PlanEntries(ctx); err != nil {
		log.Error().Err(err).Msg("cannot get plan entries for joint enrichment")
	} else {
		for _, entry := range entries {
			planEntries[entry.Ancode] = entry
		}
	}

	for _, exam := range exams {
		link := linkByKey[primussKey{exam.Program, exam.PrimussAncode}]
		if link == nil || link.Status == "unresolved" {
			exam.LinkStatus = jointLinkUnresolved
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

func (p *Plexams) JointExamsForProgram(ctx context.Context, program string) ([]*model.JointExam, error) {
	exams, err := p.dbClient.JointExamsForProgram(ctx, program)
	if err != nil {
		return nil, err
	}
	jointExams := make([]*model.JointExam, 0, len(exams))
	for _, exam := range exams {
		jointExams = append(jointExams, p.mkJointExam(exam))
	}

	return jointExams, nil
}

func (p *Plexams) JointExam(ctx context.Context, program string, ancode int) (*model.JointExam, error) {
	exam, err := p.dbClient.JointExam(ctx, program, ancode)
	if err != nil {
		return nil, err
	}
	return p.mkJointExam(exam), nil
}

func (p *Plexams) mkJointExam(jointExam *db.JointExam) *model.JointExam {
	isRepeaterExam := jointExam.IsRepeaterExam == "x"

	return &model.JointExam{
		PrimussAncode:  jointExam.PrimussAncode,
		Module:         jointExam.Module,
		MainExamer:     jointExam.MainExamer,
		ExamType:       jointExam.ExamType,
		Duration:       jointExam.Duration,
		IsRepeaterExam: isRepeaterExam,
		Program:        jointExam.Program,
		PlannedBy:      jointExam.Planer,
	}
}

// AddJointExamByProgram adds a MUC.DAI exam, deriving the local ZPA ancode from the
// program's externalExamsBase (base + primussAncode) in the StudyProgram master data.
func (p *Plexams) AddJointExamByProgram(ctx context.Context, jointExam *model.JointExam) (*model.ZPAExam, error) {
	base, ok := p.externalExamsBaseForProgram(ctx, jointExam.Program)
	if !ok {
		return nil, fmt.Errorf("no externalExamsBase set for program %s (StudyProgram master data)", jointExam.Program)
	}
	return p.AddJointExam(ctx, base+jointExam.PrimussAncode, jointExam)
}

func (p *Plexams) AddJointExam(ctx context.Context, zpaAncode int, jointExam *model.JointExam) (*model.ZPAExam, error) {
	zpaExam := &model.ZPAExam{
		ZpaID:          0,
		Semester:       p.semester,
		AnCode:         zpaAncode,
		Module:         jointExam.Module,
		MainExamer:     jointExam.MainExamer,
		MainExamerID:   0,
		ExamType:       jointExam.ExamType,
		ExamTypeFull:   "",
		Date:           "",
		Starttime:      "",
		Duration:       jointExam.Duration,
		IsRepeaterExam: jointExam.IsRepeaterExam,
		Groups:         []string{},
		PrimussAncodes: []model.ZPAPrimussAncodes{{
			Program: jointExam.Program,
			Ancode:  jointExam.PrimussAncode,
		}},
	}
	// The faculty is a per-exam fact from the MUC.DAI CSV (Prüfungsplanung), not a
	// property of the program: within one program the exams split across faculties
	// (e.g. in DE some are FK07's, others FK03's). FK07-planned exams never reach here
	// (they go through the normal ZPA flow), so PlannedBy is the other faculty.
	zpaExam.Faculty = strings.TrimSpace(jointExam.PlannedBy)

	err := p.dbClient.AddExternalExam(ctx, zpaExam)

	return zpaExam, err
}
