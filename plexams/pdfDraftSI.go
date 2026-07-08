package plexams

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"

	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/pdfgen"
	"github.com/rs/zerolog/log"
)

// lbaRepExams collects the repeater exams whose main examer is an LBA (excluding
// exams not planned by me).
func (p *Plexams) lbaRepExams(ctx context.Context) []*model.PlannedExam {
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
	return exams
}

func (p *Plexams) draftSIMaroto(name string, exams []*model.PlannedExam) pdf.Maroto {
	m := pdfgen.DraftDoc(false,
		fmt.Sprintf("Vorläufiger Planungsstand der Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- ENTWURF ---")

	pdfgen.ExamTable(m, name, pdfgen.ExamRows(exams))

	return m
}

// DraftLbaRepBytes builds the repeater-exams-of-LBAs draft PDF as bytes (REST download).
func (p *Plexams) DraftLbaRepBytes(ctx context.Context) ([]byte, error) {
	return marotoBytes(p.draftSIMaroto("Wiederholungsprüfungen von LBAs", p.lbaRepExams(ctx)))
}

// DraftSIZipBytes builds one draft PDF per special interest and returns them as a ZIP,
// since the special-interest export produces several files (one per configured group).
func (p *Plexams) DraftSIZipBytes(ctx context.Context) ([]byte, error) {
	sis, err := p.dbClient.SpecialInterests(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get special interests")
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, si := range sis {
		exams := make([]*model.PlannedExam, 0, len(si.Ancodes))
		for _, ancode := range si.Ancodes {
			exam, err := p.PlannedExam(ctx, ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exams with ancode")
				continue
			}
			exams = append(exams, exam)
		}
		data, err := marotoBytes(p.draftSIMaroto(si.Name, exams))
		if err != nil {
			return nil, fmt.Errorf("special interest %q: %w", si.Name, err)
		}
		f, err := zw.Create(si.Filename)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
