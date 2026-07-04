package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/pdfgen"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) DraftSI(ctx context.Context) error {
	sis, err := p.dbClient.SpecialInterests(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get special interests")
		return err
	}

	for _, si := range sis {
		log.Debug().Str("name", si.Name).Str("filename", si.Filename).
			Interface("ancodes", si.Ancodes).Msg("found special interest")

		exams := make([]*model.PlannedExam, 0, len(si.Ancodes))
		for _, ancode := range si.Ancodes {
			exam, err := p.PlannedExam(ctx, ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exams with ancode")
				continue
			}
			exams = append(exams, exam)
		}

		if err := p.draftSI(si.Name, si.Filename, exams); err != nil {
			log.Error().Err(err).Msg("cannot draft SI")
		}
	}

	return nil
}

func (p *Plexams) DraftLbaRep(ctx context.Context) error {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
	}
	exams := make([]*model.PlannedExam, 0)
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		if exam.ZpaExam.IsRepeaterExam {
			examer, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
			if err != nil {
				log.Error().Err(err).Int("main examer ID", exam.ZpaExam.MainExamerID).Msg("cannot get teacher")
			}
			if examer.IsLBA {
				exams = append(exams, exam)
			}
		}
	}
	return p.draftSI("Wiederholungsprüfungen von LBAs", "draft-lba-rep.pdf", exams)
}

func (p *Plexams) draftSI(name string, outfile string, exams []*model.PlannedExam) error {
	m := pdfgen.DraftDoc(false,
		fmt.Sprintf("Vorläufiger Planungsstand der Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- ENTWURF ---")

	pdfgen.ExamTable(m, name, pdfgen.ExamRows(exams, p.getSlotTime))

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	} else {
		fmt.Printf("generated %s for %s\n", outfile, name)
	}
	return nil
}
