package pdfgen

import (
	"fmt"
	"strings"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
)

// SpreadStatistics renders the aggregate exam-spread statistics PDF (portrait). It is
// deliberately anonymous: only aggregated figures, distributions and per-program
// numbers — no student names — so it can be emailed on to the Prüfungsamt / faculty.
func SpreadStatistics(semesterFull string, stat *model.ExamSpreadStatistics) pdf.Maroto {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)
	footer(m)
	gray := color.Color{Red: 211, Green: 211, Blue: 211}

	centeredRow(m, 10, 3, consts.Bold,
		fmt.Sprintf("Zeitliche Verteilung der Prüfungen für die Studierenden — %s", semesterFull))
	centeredRow(m, 10, 2, consts.Normal,
		"Abstände zwischen aufeinanderfolgenden Prüfungen je Studierende:r. „Freier Tag“ zählt Kalendertage (Wochenende inkl.).")

	// --- Kennzahlen -----------------------------------------------------------
	sectionRow(m, "Kennzahlen")
	m.TableList([]string{"Kennzahl", "Wert"}, spreadKeyFigureRows(stat), props.TableList{
		HeaderProp:           props.TableListContent{Size: 9, GridSizes: []uint{9, 3}},
		ContentProp:          props.TableListContent{Size: 9, GridSizes: []uint{9, 3}},
		Align:                consts.Left,
		AlternatedBackground: &gray,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	// --- Verteilung -----------------------------------------------------------
	sectionRow(m, "Verteilung der Studierenden nach engstem Prüfungsabstand")
	m.TableList([]string{"Kategorie", "Studierende", "Anteil", ""}, spreadBucketRows(stat.StudentBuckets), props.TableList{
		HeaderProp:           props.TableListContent{Size: 9, GridSizes: []uint{5, 2, 2, 3}},
		ContentProp:          props.TableListContent{Size: 9, GridSizes: []uint{5, 2, 2, 3}},
		Align:                consts.Left,
		AlternatedBackground: &gray,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	// --- pro Studiengang ------------------------------------------------------
	sectionRow(m, "Nach Studiengang (mind. 1 freier Tag / selber Tag, bezogen auf Studierende mit ≥ 2 Prüfungen)")
	m.TableList(
		[]string{"Studiengang", "Studierende", "≥2 Prüf.", "Ø Prüf.", "≥1 frei", "selber Tag", "Ø min. frei"},
		spreadProgramRows(stat.ByProgram),
		props.TableList{
			HeaderProp:           props.TableListContent{Size: 8, GridSizes: []uint{3, 2, 2, 1, 2, 1, 1}},
			ContentProp:          props.TableListContent{Size: 8, GridSizes: []uint{3, 2, 2, 1, 2, 1, 1}},
			Align:                consts.Left,
			AlternatedBackground: &gray,
			HeaderContentSpace:   1,
			Line:                 false,
		})
	if anyLowSample(stat.ByProgram) {
		centeredRow(m, 8, 2, consts.Italic,
			fmt.Sprintf("* geringe Fallzahl (< %d Studierende mit ≥ 2 Prüfungen) — Anteile mit Vorsicht lesen.", spreadLowSampleThreshold))
	}

	// --- Prüfungen pro Studierende:r -----------------------------------------
	sectionRow(m, "Prüfungen je Studierende:r")
	m.TableList([]string{"Anzahl Prüfungen", "Studierende", "Anteil"}, spreadExamCountRows(stat.ExamCountBuckets), props.TableList{
		HeaderProp:           props.TableListContent{Size: 9, GridSizes: []uint{6, 3, 3}},
		ContentProp:          props.TableListContent{Size: 9, GridSizes: []uint{6, 3, 3}},
		Align:                consts.Left,
		AlternatedBackground: &gray,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	if stat.StudentsWithUnplannedExams > 0 {
		centeredRow(m, 10, 4, consts.Italic,
			fmt.Sprintf("Hinweis: %d Studierende haben noch mindestens eine nicht verplante Prüfung — die Zahlen decken nur die bereits geplanten Prüfungen ab.",
				stat.StudentsWithUnplannedExams))
	}

	return m
}

// sectionRow renders a bold left-aligned section heading.
func sectionRow(m pdf.Maroto, title string) {
	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(title, props.Text{Top: 4, Style: consts.Bold, Size: 10, Align: consts.Left})
		})
	})
}

func pct(v float64) string  { return fmt.Sprintf("%.1f %%", v) }
func days(v float64) string { return fmt.Sprintf("%.1f", v) }

// spreadKeyFigureRows builds the headline key-figure table (label, value).
func spreadKeyFigureRows(s *model.ExamSpreadStatistics) [][]string {
	return [][]string{
		{"Studierende mit geplanten Prüfungen", fmt.Sprintf("%d", s.StudentCount)},
		{"davon mit mindestens 2 Prüfungen", fmt.Sprintf("%d", s.MultiExamStudentCount)},
		{"Ø Prüfungen je Studierende:r", fmt.Sprintf("%.2f", s.AvgExamsPerStudent)},
		{"höchste Prüfungszahl einer/eines Studierenden", fmt.Sprintf("%d", s.MaxExamsPerStudent)},
		{"mindestens 1 freier Tag zwischen ALLEN Prüfungen", pct(s.FreeDayShare)},
		{"zwei Prüfungen am selben Tag", pct(s.SameDayShare)},
		{"aufeinanderfolgende Tage (kein freier Tag)", pct(s.AdjacentDayShare)},
		{"Überschneidung / Konflikt (sollte 0 sein)", pct(s.ConflictShare)},
		{"Studierende mit ≥ 3 Prüfungen an einem Tag", fmt.Sprintf("%d", s.ThreeExamsOneDayCount)},
		{"Ø kleinster Abstand (freie Tage; selber Tag = -1)", days(s.AvgMinFreeDays)},
		{"Median kleinster Abstand (freie Tage)", days(s.MedianMinFreeDays)},
		{"Carter-Näherungsindex (Ø je Studierende:r, kleiner = besser)", days(s.AvgProximityCost)},
	}
}

// spreadBucketRows builds the distribution rows with a simple text bar.
func spreadBucketRows(buckets []*model.SpreadBucket) [][]string {
	rows := make([][]string, 0, len(buckets))
	for _, b := range buckets {
		rows = append(rows, []string{b.Label, fmt.Sprintf("%d", b.Count), pct(b.Share), bar(b.Share)})
	}
	return rows
}

// bar returns a proportional block bar (full width = 100 %).
func bar(share float64) string {
	n := int(share/4 + 0.5) // up to 25 blocks
	if n == 0 && share > 0 {
		n = 1
	}
	return strings.Repeat("█", n)
}

// spreadLowSampleThreshold mirrors plexams.lowSampleThreshold for the footnote text;
// the per-row flag itself comes from the model (ProgramSpread.LowSampleSize).
const spreadLowSampleThreshold = 5

// anyLowSample reports whether any program row is flagged as low-sample.
func anyLowSample(progs []*model.ProgramSpread) bool {
	for _, p := range progs {
		if p.LowSampleSize {
			return true
		}
	}
	return false
}

func spreadProgramRows(progs []*model.ProgramSpread) [][]string {
	rows := make([][]string, 0, len(progs))
	for _, p := range progs {
		name := p.Program
		if p.LowSampleSize {
			name += " *"
		}
		rows = append(rows, []string{
			name,
			fmt.Sprintf("%d", p.StudentCount),
			fmt.Sprintf("%d", p.MultiExamStudentCount),
			fmt.Sprintf("%.1f", p.AvgExamsPerStudent),
			pct(p.FreeDayShare),
			pct(p.SameDayShare),
			days(p.AvgMinFreeDays),
		})
	}
	return rows
}

func spreadExamCountRows(buckets []*model.CountBucket) [][]string {
	rows := make([][]string, 0, len(buckets))
	for _, b := range buckets {
		rows = append(rows, []string{b.Label, fmt.Sprintf("%d", b.Students), pct(b.Share)})
	}
	return rows
}
