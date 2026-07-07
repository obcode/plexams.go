package plexams

import (
	"context"
	"fmt"

	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/pdfgen"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) DraftMucDaiPDF(ctx context.Context, outfile string) error {
	m := pdfgen.DraftDoc(false,
		fmt.Sprintf("Vorläufiger Planungsstand MUC.DAI-Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- zur Abstimmung ---")

	p.tableForProgram(ctx, "DE", "Digital Engineering (DE)", m)
	p.tableForProgram(ctx, "ID", "Informatik und Design (ID)", m)
	p.tableForProgram(ctx, "GS", "Geodata Science (GS)", m)

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) DraftFk08PDF(ctx context.Context, outfile string) error {
	m := pdfgen.DraftDoc(false,
		fmt.Sprintf("Vorläufiger Planungsstand Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- zur Abstimmung ---")

	p.tableForProgram(ctx, "GN", "Geoinformatik und Navigation (GN)", m)
	p.tableForProgram(ctx, "GS", "Geodata Science (GS)", m)
	// p.tableForProgram(ctx, "GD", "Angewandte Geodäsie und Geoinformatik (GD)", m)

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) DraftFk10PDF(ctx context.Context, outfile string) error {
	m := pdfgen.DraftDoc(false,
		fmt.Sprintf("Vorläufiger Planungsstand Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- zur Abstimmung ---")

	p.tableForProgram(ctx, "IB", "BA - Wirtschaftsinformatik (IB)", m)
	p.tableForProgram(ctx, "IN", "MA - Wirtschaftsinformatik (IN)", m)
	p.tableForProgram(ctx, "WD", "BA - Wirtschaftsinformatik - Digitales Management (WD)", m)
	p.tableForProgram(ctx, "WT", "BA - Wirtschaftsinformatik - Informationstechnologie (WT)", m)

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) tableForProgram(ctx context.Context, program, programLong string, m pdf.Maroto) {
	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
	}
	pdfgen.ProgramTable(m, programLong, pdfgen.ProgramRows(exams, program))
}

func (p *Plexams) DraftExahmPDF(ctx context.Context, outfile string) error {
	m := pdfgen.DraftDoc(true,
		fmt.Sprintf("Vorläufiger Planungsstand Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- zur Abstimmung ---")

	// p.tableForExahm(ctx, m, false)
	p.tableForExahm(ctx, m, true)

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) tableForExahm(ctx context.Context, m pdf.Maroto, sortByDate bool) {
	allExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
	}

	exams := make([]*model.PlannedExam, 0)
	for _, exam := range allExams {
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil &&
			(exam.Constraints.RoomConstraints.Exahm || exam.Constraints.RoomConstraints.Seb) {
			exams = append(exams, exam)
		}
	}

	// Pre-resolve the pre-planned rooms (only needed for exams without planned rooms),
	// keeping the render I/O-free.
	prePlannedRooms := make(map[int][]string)
	for _, exam := range exams {
		if len(exam.PlannedRooms) > 0 {
			continue
		}
		rooms, err := p.PrePlannedRoomsForExam(ctx, exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Ancode).
				Msg("error while trying to get preplanned rooms")
		}
		names := make([]string, 0, len(rooms))
		for _, room := range rooms {
			names = append(names, room.RoomName)
		}
		prePlannedRooms[exam.Ancode] = names
	}

	pdfgen.ExahmTable(m, sortByDate, pdfgen.ExahmRows(exams, sortByDate, prePlannedRooms))
}
