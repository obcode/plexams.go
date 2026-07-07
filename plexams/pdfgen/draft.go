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
// footer, a bold title, the planner line and a centered note (e.g. "--- zur Abstimmung
// ---" or "--- ENTWURF ---"). landscape selects the page orientation (the EXaHM draft is
// landscape, the rest portrait).
func DraftDoc(landscape bool, title, planerName, planerEmail, note string) pdf.Maroto {
	orientation := consts.Portrait
	if landscape {
		orientation = consts.Landscape
	}
	m := pdf.NewMaroto(orientation, consts.A4)
	m.SetPageMargins(10, 15, 10)
	footer(m)

	draftRow(m, 6, consts.Bold, title)
	draftRow(m, 6, consts.Normal, planerName+" <"+planerEmail+">")
	draftRow(m, 15, consts.Normal, note)
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
func ProgramRows(exams []*model.PlannedExam, program string) [][]string {
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
		if exam.PlanEntry != nil && exam.PlanEntry.Starttime != nil {
			termin = FormatTermin(*exam.PlanEntry.Starttime)
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

// ExamRows builds the simple (AnCode, Modul, Prüfender, Termin) draft table rows sorted
// by ancode. An exam without a plan entry shows "fehlt noch"; slotTime resolves a plan
// entry's (day, slot) to its start. Used by the special-interest / LBA-repeater drafts.
func ExamRows(exams []*model.PlannedExam) [][]string {
	examsMap := make(map[int]*model.PlannedExam)
	ancodes := make([]int, 0, len(exams))
	for _, exam := range exams {
		examsMap[exam.Ancode] = exam
		ancodes = append(ancodes, exam.Ancode)
	}
	sort.Ints(ancodes)

	contents := make([][]string, 0, len(ancodes))
	for _, ancode := range ancodes {
		exam := examsMap[ancode]
		termin := "fehlt noch"
		if exam.PlanEntry != nil && exam.PlanEntry.Starttime != nil {
			termin = FormatTermin(*exam.PlanEntry.Starttime)
		}
		contents = append(contents, []string{strconv.Itoa(ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer, termin})
	}
	return contents
}

// ExamTable renders the named (AnCode, Modul, Prüfender, Termin) draft table into m.
func ExamTable(m pdf.Maroto, name string, rows [][]string) {
	m.Row(18, func() {
		m.Col(12, func() {
			m.Text(name, props.Text{Top: 10, Size: 12, Style: consts.Bold})
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

// ExahmHeading returns the EXaHM/SEB table heading, which names the sort order.
func ExahmHeading(sortByDate bool) string {
	if sortByDate {
		return "Prüfungen mit EXaHM/SEB, sortiert nach Datum"
	}
	return "Prüfungen mit EXaHM/SEB, sortiert nach AnCode"
}

// ExahmRows builds the EXaHM/SEB draft table rows (AnCode, Modul, Prüfender, Termin,
// Form, Plätze, Räume). When sortByDate the exams are ordered by (day, slot, ancode),
// otherwise the input order is kept. slotTime resolves a plan entry to its start;
// prePlannedRooms maps an ancode to its pre-planned room names, used only when the exam
// has no planned rooms yet. Every exam must already carry non-nil Constraints.RoomConstraints
// (the caller filters to EXaHM/SEB exams).
func ExahmRows(exams []*model.PlannedExam, sortByDate bool,
	prePlannedRooms map[int][]string) [][]string {
	sorted := make([]*model.PlannedExam, len(exams))
	copy(sorted, exams)
	if sortByDate {
		sort.Slice(sorted, func(i, j int) bool {
			si, sj := sorted[i].PlanEntry, sorted[j].PlanEntry
			if si == nil || si.Starttime == nil {
				return false
			}
			if sj == nil || sj.Starttime == nil {
				return true
			}
			if !si.Starttime.Equal(*sj.Starttime) {
				return si.Starttime.Before(*sj.Starttime)
			}
			return sorted[i].Ancode < sorted[j].Ancode
		})
	}

	contents := make([][]string, 0, len(sorted))
	for _, exam := range sorted {
		termin := "fehlt noch"
		if exam.PlanEntry != nil && exam.PlanEntry.Starttime != nil {
			termin = FormatTermin(*exam.PlanEntry.Starttime)
		}

		rooms := "fehlen noch"
		if len(exam.PlannedRooms) > 0 {
			names := make([]string, 0, len(exam.PlannedRooms))
			for _, room := range exam.PlannedRooms {
				names = append(names, room.RoomName)
			}
			rooms = strings.Join(names, ", ")
		} else if prePlanned := prePlannedRooms[exam.Ancode]; len(prePlanned) > 0 {
			rooms = strings.Join(prePlanned, ", ")
		}

		variant := "SEB"
		if exam.Constraints.RoomConstraints.Exahm {
			variant = "EXaHM"
		}

		contents = append(contents, []string{
			strconv.Itoa(exam.Ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
			termin, variant, strconv.Itoa(exam.StudentRegsCount), rooms})
	}
	return contents
}

// ExahmTable renders the EXaHM/SEB heading and table into m.
func ExahmTable(m pdf.Maroto, sortByDate bool, rows [][]string) {
	m.Row(18, func() {
		m.Col(12, func() {
			m.Text(ExahmHeading(sortByDate), props.Text{Top: 10, Size: 12, Style: consts.Bold})
		})
	})

	grayColor := color.Color{Red: 211, Green: 211, Blue: 211}
	m.TableList([]string{"AnCode", "Modul", "Prüfender", "Termin", "Form", "Plätze", "Räume"}, rows, props.TableList{
		HeaderProp:           props.TableListContent{Size: 11, GridSizes: []uint{1, 3, 2, 3, 1, 1, 1}},
		ContentProp:          props.TableListContent{Size: 11, GridSizes: []uint{1, 3, 2, 3, 1, 1, 1}},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})
}
