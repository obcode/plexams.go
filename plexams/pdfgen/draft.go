package pdfgen

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
)

// dayReplacer shortens the English weekday abbreviations maroto/time produce to the
// German two-letter forms (Mon->Mo, Tue->Di, …).
var dayReplacer = strings.NewReplacer(
	"Mon", "Mo", "Tue", "Di", "Wed", "Mi", "Thu", "Do", "Fri", "Fr", "Sat", "Sa", "Sun", "So",
)

// FormatTermin renders a slot start time as e.g. "Mo. 02.01.06, 15:04 Uhr".
func FormatTermin(t time.Time) string {
	return dayReplacer.Replace(t.Format("Mon. 02.01.06, 15:04 Uhr"))
}

// DraftDoc builds the shared "vorläufiger Planungsstand" draft document shell: the
// footer, a bold title, the planner line and the "zur Abstimmung" note. landscape
// selects the page orientation (the EXaHM draft is landscape, the rest portrait).
func DraftDoc(landscape bool, title, planerName, planerEmail string) pdf.Maroto {
	orientation := consts.Portrait
	if landscape {
		orientation = consts.Landscape
	}
	m := pdf.NewMaroto(orientation, consts.A4)
	m.SetPageMargins(10, 15, 10)
	footer(m)

	draftRow(m, 6, consts.Bold, title)
	draftRow(m, 6, consts.Normal, planerName+" <"+planerEmail+">")
	draftRow(m, 15, consts.Normal, "--- zur Abstimmung ---")
	return m
}

// draftRow adds a size-12 centered text row with the draft-header layout (Top 3).
func draftRow(m pdf.Maroto, height float64, style consts.Style, text string) {
	m.Row(height, func() {
		m.Col(12, func() {
			m.Text(text, props.Text{Top: 3, Size: 12, Style: style, Align: consts.Center})
		})
	})
}

// ProgramRows builds the per-program draft table rows (AnCode, Modul, Prüfender,
// Termin) sorted by ancode. For an exam with a Primuss section in this program the
// section's ancode is used; a section with no registrations stops the listing (matching
// the original behaviour). slotTime resolves a plan entry's (day, slot) to its start.
func ProgramRows(exams []*model.PlannedExam, program string, slotTime func(day, slot int) time.Time) [][]string {
	contentsMap := make(map[int][]string)
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

		termin := "fehlt noch"
		if exam.PlanEntry != nil {
			termin = FormatTermin(slotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber))
		}
		contentsMap[ancode] = []string{strconv.Itoa(ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer, termin}
	}

	sort.Ints(ancodes)
	contents := make([][]string, 0, len(contentsMap))
	for _, ancode := range ancodes {
		contents = append(contents, contentsMap[ancode])
	}
	return contents
}

// ProgramTable renders one program's heading and its exam table into m.
func ProgramTable(m pdf.Maroto, programLong string, rows [][]string) {
	m.Row(18, func() {
		m.Col(12, func() {
			m.Text(programLong, props.Text{Top: 10, Size: 12, Style: consts.Bold})
		})
	})

	grayColor := color.Color{Red: 211, Green: 211, Blue: 211}
	m.TableList([]string{"AnCode", "Modul", "Prüfender", "Termin"}, rows, props.TableList{
		HeaderProp:           props.TableListContent{Size: 11, GridSizes: []uint{1, 5, 2, 4}},
		ContentProp:          props.TableListContent{Size: 11, GridSizes: []uint{1, 5, 2, 4}},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})
}
