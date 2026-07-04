package pdfgen

import (
	"fmt"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
)

// footer registers the shared "Stand: … generiert mit …" page footer.
func footer(m pdf.Maroto) {
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
}

// centeredRow adds a full-width centered text row of the given height.
func centeredRow(m pdf.Maroto, height float64, top float64, style consts.Style, text string) {
	m.Row(height, func() {
		m.Col(12, func() {
			m.Text(text, props.Text{Top: top, Style: style, Align: consts.Center})
		})
	})
}

// ExamsToPlan renders the portrait "exams to plan" PDF (centrally planned exams,
// grouped by examer).
func ExamsToPlan(semesterFull, jiraURL string, exams []*model.ZPAExam) pdf.Maroto {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)
	footer(m)

	centeredRow(m, 10, 3, consts.Bold,
		fmt.Sprintf("Prüfungen, die im Prüfungszeitraum %s stattfinden und daher zentral geplant werden.", semesterFull))
	centeredRow(m, 10, 3, consts.Normal,
		fmt.Sprintf("Öffnen Sie bitte umgehend ein JIRA-Ticket unter %s, wenn Ihre Prüfung hier fehlt oder hier nicht stehen sollte.", jiraURL))
	centeredRow(m, 20, 5, consts.Bold, "Sortiert nach dem Namen des Prüferenden.")

	grayColor := color.Color{Red: 211, Green: 211, Blue: 211}
	m.TableList([]string{"AnCode", "Modul", "Prüfender", "Gruppe(n)"}, ExamsToPlanRows(exams), props.TableList{
		HeaderProp:           props.TableListContent{Size: 9, GridSizes: []uint{1, 6, 2, 3}},
		ContentProp:          props.TableListContent{Size: 8, GridSizes: []uint{1, 6, 2, 3}},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})
	return m
}

// SameModulNames renders the landscape "modules with the same name" PDF.
func SameModulNames(semesterFull string, exams []*model.ZPAExam) pdf.Maroto {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
	m.SetPageMargins(10, 15, 10)
	footer(m)

	centeredRow(m, 10, 3, consts.Bold,
		fmt.Sprintf("Module mit gleichem Namen im Prüfungszeitraum %s.", semesterFull))

	m.TableList([]string{"Modul", "AnCode", "Prüfender", "Gruppe(n)", "Form"}, SameModulNamesRows(exams), props.TableList{
		HeaderProp:         props.TableListContent{Size: 9, GridSizes: []uint{4, 1, 2, 2, 3}},
		ContentProp:        props.TableListContent{Size: 8, GridSizes: []uint{4, 1, 2, 2, 3}},
		Align:              consts.Left,
		HeaderContentSpace: 1,
		Line:               true,
	})
	return m
}

// Constraints renders the landscape constraints PDF. sameSlotExam resolves the
// same-slot partner ancodes (pre-fetched by the caller; see SameSlotAncodes).
func Constraints(semesterFull, jiraURL string, exams []*model.ZPAExamWithConstraints, sameSlotExam map[int]*model.ZPAExam) pdf.Maroto {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
	m.SetPageMargins(10, 15, 10)
	footer(m)

	centeredRow(m, 10, 3, consts.Bold,
		fmt.Sprintf("Constraints für den Prüfungszeitraum %s.", semesterFull))
	centeredRow(m, 12, 3, consts.Normal,
		fmt.Sprintf("Öffnen Sie bitte umgehend ein JIRA-Ticket unter %s, wenn Ihre Prüfung eine Randbedingung hat, die hier fehlt oder hier nicht stehen sollte.", jiraURL))
	centeredRow(m, 12, 0, consts.Normal,
		`Für alle Prüfungen, die hier nicht enthalten sind, sind uns keine Einschränkungen für die Planung bekannt.`)
	centeredRow(m, 20, 5, consts.Bold, "Sortiert nach dem Namen des Prüferenden.")

	m.TableList([]string{"AnCode", "Prüfender", "Modul", "Gruppe(n)", "Constraints"}, ConstraintsRows(exams, sameSlotExam), props.TableList{
		HeaderProp:         props.TableListContent{Size: 9, GridSizes: []uint{1, 1, 3, 2, 5}},
		ContentProp:        props.TableListContent{Size: 8, GridSizes: []uint{1, 1, 3, 2, 5}},
		Align:              consts.Left,
		HeaderContentSpace: 1,
		Line:               true,
	})
	return m
}
