package plexams

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
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
