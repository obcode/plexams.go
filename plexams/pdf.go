package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GenerateExamsToPlanPDF(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
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

	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Prüfungen, die im Prüfungszeitraum %s stattfinden und daher zentral geplant werden.", p.semester), props.Text{
					Top:   3,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(20, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Melden Sie sich bitte umgehend per E-Mail (%s) bei mir (%s), wenn Ihre Prüfung hier fehlt oder hier nicht stehen sollte.",
					p.planer.Email, p.planer.Name), props.Text{
					Top:   3,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	header := []string{"AnCode", "Modul", "Prüfer:in", "Gruppe(n)", "Form"}

	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
	}

	contents := make([][]string, 0, len(exams))

	for _, exam := range exams {
		contents = append(contents, []string{strconv.Itoa(exam.AnCode), exam.Module, exam.MainExamer, fmt.Sprintf("%v", exam.Groups), exam.ExamTypeFull})
	}

	grayColor := color.Color{
		Red:   211,
		Green: 211,
		Blue:  211,
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) SameModulNames(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
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

	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Module mit gleichem Namen im Prüfungszeitraum %s.", p.semester), props.Text{
					Top:   3,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})

	header := []string{"Modul", "AnCode", "Prüfer:in", "Gruppe(n)", "Form"}

	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	sameModules := make(map[string][]*model.ZPAExam)
	for _, exam := range exams {
		same, ok := sameModules[exam.Module]
		if ok {
			sameModules[exam.Module] = append(same, exam)
		} else {
			sameModules[exam.Module] = []*model.ZPAExam{exam}
		}
	}

	names := make([]string, 0, len(sameModules))
	for name := range sameModules {
		names = append(names, name)
	}

	sort.Strings(names)

	contents := make([][]string, 0, len(exams))

	for _, module := range names {
		exams := sameModules[module]
		ancode := strconv.Itoa(exams[0].AnCode)
		examiner := exams[0].MainExamer
		group := fmt.Sprintf("%v", exams[0].Groups)
		examType := exams[0].ExamTypeFull

		contents = append(contents, []string{module, ancode, examiner, group, examType})

		for _, exam := range exams[1:] {
			ancode := strconv.Itoa(exam.AnCode)
			examiner := exam.MainExamer
			group := fmt.Sprintf("%v", exam.Groups)
			examType := exam.ExamTypeFull

			contents = append(contents, []string{"", ancode, examiner, group, examType})
		}

		contents = append(contents, []string{"", "", "", "", ""})

	}

	// grayColor := color.Color{
	// 	Red:   211,
	// 	Green: 211,
	// 	Blue:  211,
	// }

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{4, 1, 2, 2, 3},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{4, 1, 2, 2, 3},
		},
		Align: consts.Left,
		// AlternatedBackground: &grayColor,
		HeaderContentSpace: 1,
		Line:               true,
	})

	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}
