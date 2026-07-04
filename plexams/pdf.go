package plexams

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/pdfgen"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) semesterFull() string {
	s := strings.Split(p.semester, " ")
	year := s[0]
	sem := s[1]
	full := ""

	if sem == "SS" {
		full = fmt.Sprint("Sommersemester ", year)
	} else {
		yearInt, _ := strconv.Atoi(year)
		full = fmt.Sprintf("Wintersemester %d/%d", yearInt, yearInt-1999)
	}

	return full
}

func (p *Plexams) GenerateExamsToPlanPDF(ctx context.Context, outfile string) error {
	m, err := p.generateExamsToPlanMaroto(ctx)
	if err != nil {
		return err
	}
	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) generateExamsToPlanMaroto(ctx context.Context) (pdf.Maroto, error) {
	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
		return nil, err
	}
	return pdfgen.ExamsToPlan(p.semesterFull(), jiraURL(), exams), nil
}

func (p *Plexams) SameModulNames(ctx context.Context, outfile string) error {
	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}
	m := pdfgen.SameModulNames(p.semesterFull(), exams)
	if err := m.OutputFileAndClose(outfile); err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) ConstraintsPDF(ctx context.Context, outfile string) error {
	m, err := p.constraintsMaroto(ctx)
	if err != nil {
		return err
	}
	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) constraintsMaroto(ctx context.Context) (pdf.Maroto, error) {
	examsWithConstraints, err := p.ZpaExamsToPlanWithConstraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
		return nil, err
	}

	// resolve the same-slot partner exams up front so the renderer stays I/O-free
	sameSlotExam := make(map[int]*model.ZPAExam)
	for _, ancode := range pdfgen.SameSlotAncodes(examsWithConstraints) {
		otherExam, err := p.GetZpaExamByAncode(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam for same-slot ancode")
		}
		sameSlotExam[ancode] = otherExam
	}

	return pdfgen.Constraints(p.semesterFull(), jiraURL(), examsWithConstraints, sameSlotExam), nil
}
