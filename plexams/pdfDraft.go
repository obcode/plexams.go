package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

var r = strings.NewReplacer(
	"Mon", "Mo",
	"Tue", "Di",
	"Wed", "Mi",
	"Thu", "Do",
	"Fri", "Fr",
	"Sat", "Sa",
	"Sun", "So",
)

func (p *Plexams) DraftMucDaiPDF(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)

	m.RegisterFooter(func() {
		m.Row(20, func() {
			m.Col(12, func() {
				m.Text(fmt.Sprintf("Stand: %s Uhr, generiert mit https://github.com/obcode/plexams.go",
					time.Now().Format("02.01.06, 15:04")), props.Text{
					Top:   13,
					Style: consts.BoldItalic,
					Size:  8,
					Align: consts.Left,
				})
			})
		})
	})

	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Vorläufiger Planungsstand MUC.DAI-Prüfungen der FK07 im %s", p.semesterFull()), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(15, func() {
		m.Col(12, func() {
			m.Text(
				"--- zur Abstimmung ---", props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

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
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)

	m.RegisterFooter(func() {
		m.Row(20, func() {
			m.Col(12, func() {
				m.Text(fmt.Sprintf("Stand: %s Uhr, generiert mit https://github.com/obcode/plexams.go",
					time.Now().Format("02.01.06, 15:04")), props.Text{
					Top:   13,
					Style: consts.BoldItalic,
					Size:  8,
					Align: consts.Left,
				})
			})
		})
	})

	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Vorläufiger Planungsstand Prüfungen der FK07 im %s", p.semesterFull()), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(15, func() {
		m.Col(12, func() {
			m.Text(
				"--- zur Abstimmung ---", props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

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
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)

	m.RegisterFooter(func() {
		m.Row(20, func() {
			m.Col(12, func() {
				m.Text(fmt.Sprintf("Stand: %s Uhr, generiert mit https://github.com/obcode/plexams.go",
					time.Now().Format("02.01.06, 15:04")), props.Text{
					Top:   13,
					Style: consts.BoldItalic,
					Size:  8,
					Align: consts.Left,
				})
			})
		})
	})

	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Vorläufiger Planungsstand Prüfungen der FK07 im %s", p.semesterFull()), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(15, func() {
		m.Col(12, func() {
			m.Text(
				"--- zur Abstimmung ---", props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

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
	header := []string{"AnCode", "Modul", "Prüfender", "Termin"}

	m.Row(18, func() {
		m.Col(12, func() {
			m.Text(
				programLong, props.Text{
					Top:   10,
					Size:  12,
					Style: consts.Bold,
				})
		})
	})

	contentsMap := make(map[int][]string)

	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
	}
	ancodes := make([]int, 0, len(exams))

OUTER:
	for _, exam := range exams {
		ancode := exam.Ancode

		for _, primussExam := range exam.PrimussExams {
			if primussExam.Exam.Program == program {
				if len(primussExam.StudentRegs) == 0 {
					break OUTER
				}
				ancode = primussExam.Exam.AnCode
			}
		}

		ancodes = append(ancodes, ancode)

		if exam.PlanEntry == nil {
			contentsMap[ancode] =
				[]string{strconv.Itoa(ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
					"fehlt noch"}
		} else {
			starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			contentsMap[ancode] =
				[]string{strconv.Itoa(ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
					r.Replace(starttime.Local().Format("Mon. 02.01.06, 15:04 Uhr"))}
		}
	}

	sort.Ints(ancodes)

	contents := make([][]string, 0, len(contentsMap))
	for _, ancode := range ancodes {
		contents = append(contents, contentsMap[ancode])
	}

	grayColor := color.Color{
		Red:   211,
		Green: 211,
		Blue:  211,
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      11,
			GridSizes: []uint{1, 5, 2, 4},
		},
		ContentProp: props.TableListContent{
			Size:      11,
			GridSizes: []uint{1, 5, 2, 4},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

}

func (p *Plexams) DraftExahmPDF(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)

	m.RegisterFooter(func() {
		m.Row(20, func() {
			m.Col(12, func() {
				m.Text(fmt.Sprintf("Stand: %s Uhr, generiert mit https://github.com/obcode/plexams.go",
					time.Now().Format("02.01.06, 15:04")), props.Text{
					Top:   13,
					Style: consts.BoldItalic,
					Size:  8,
					Align: consts.Left,
				})
			})
		})
	})

	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Vorläufiger Planungsstand Prüfungen der FK07 im %s", p.semesterFull()), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(15, func() {
		m.Col(12, func() {
			m.Text(
				"--- zur Abstimmung ---", props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	p.tableForExahm(ctx, m, false)
	p.tableForExahm(ctx, m, true)

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) tableForExahm(ctx context.Context, m pdf.Maroto, sortByDate bool) {
	header := []string{"AnCode", "Modul", "Prüfender", "Termin", "Plätze"}

	text := "Prüfungen mit EXaHM/SEB, sortiert nach AnCode"
	if sortByDate {
		text = "Prüfungen mit EXaHM/SEB, sortiert nach Datum"
	}

	m.Row(18, func() {
		m.Col(12, func() {
			m.Text(
				text, props.Text{
					Top:   10,
					Size:  12,
					Style: consts.Bold,
				})
		})
	})

	contents := make([][]string, 0)

	allExams, err := p.PlannedExams(ctx)
	exams := make([]*model.PlannedExam, 0)

	for _, exam := range allExams {
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil &&
			(exam.Constraints.RoomConstraints.Exahm || exam.Constraints.RoomConstraints.Seb) {
			exams = append(exams, exam)
		}
	}

	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
	}

	if sortByDate {
		sort.Slice(exams, func(i, j int) bool {
			if exams[i].PlanEntry == nil {
				return false
			}
			if exams[j].PlanEntry == nil {
				return true
			}
			return exams[i].PlanEntry.DayNumber < exams[j].PlanEntry.DayNumber ||
				(exams[i].PlanEntry.DayNumber == exams[j].PlanEntry.DayNumber &&
					exams[i].PlanEntry.SlotNumber < exams[j].PlanEntry.SlotNumber)
		})
	}

	for _, exam := range exams {
		if exam.PlanEntry == nil {
			contents = append(contents,
				[]string{strconv.Itoa(exam.Ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
					"fehlt noch", strconv.Itoa(exam.StudentRegsCount)})
		} else {
			starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			contents = append(contents,
				[]string{strconv.Itoa(exam.Ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
					r.Replace(starttime.Local().Format("Mon. 02.01.06, 15:04 Uhr")), strconv.Itoa(exam.StudentRegsCount)})
		}
	}

	grayColor := color.Color{
		Red:   211,
		Green: 211,
		Blue:  211,
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      11,
			GridSizes: []uint{1, 5, 2, 3, 1},
		},
		ContentProp: props.TableListContent{
			Size:      11,
			GridSizes: []uint{1, 5, 2, 3, 1},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

}
