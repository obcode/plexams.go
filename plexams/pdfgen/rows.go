// Package pdfgen renders the planning PDFs (maroto documents) from already-fetched
// data. The plexams package gathers the data from the DB and passes it in; the pure
// table-content builders here (…Rows) are I/O-free and unit-tested, and the render
// functions wrap them with the shared page furniture. Keeping the maroto layout out of
// the plexams god-package is the point.
package pdfgen

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// examsByExamer groups exams by their MainExamer and returns the examer names sorted
// ascending, so the tables are stable across runs.
func examsByExamer(exams []*model.ZPAExam) (map[string][]*model.ZPAExam, []string) {
	byExamer := make(map[string][]*model.ZPAExam)
	for _, exam := range exams {
		byExamer[exam.MainExamer] = append(byExamer[exam.MainExamer], exam)
	}
	keys := make([]string, 0, len(byExamer))
	for k := range byExamer {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return byExamer, keys
}

// ExamsToPlanRows builds the "exams to plan" table rows (AnCode, Modul, Prüfender,
// Gruppe(n)), grouped and sorted by examer name.
func ExamsToPlanRows(exams []*model.ZPAExam) [][]string {
	byExamer, keys := examsByExamer(exams)
	contents := make([][]string, 0, len(exams))
	for _, key := range keys {
		for _, exam := range byExamer[key] {
			contents = append(contents, []string{strconv.Itoa(exam.AnCode), exam.Module, exam.MainExamer, fmt.Sprintf("%v", exam.Groups)})
		}
	}
	return contents
}

// SameModulNamesRows builds the "modules with the same name" table rows (Modul, AnCode,
// Prüfender, Gruppe(n), Form): one block per module name (sorted), the module name shown
// only on the first row, followed by a blank separator row.
func SameModulNamesRows(exams []*model.ZPAExam) [][]string {
	sameModules := make(map[string][]*model.ZPAExam)
	for _, exam := range exams {
		sameModules[exam.Module] = append(sameModules[exam.Module], exam)
	}
	names := make([]string, 0, len(sameModules))
	for name := range sameModules {
		names = append(names, name)
	}
	sort.Strings(names)

	contents := make([][]string, 0, len(exams))
	for _, module := range names {
		exams := sameModules[module]
		contents = append(contents, []string{module, strconv.Itoa(exams[0].AnCode), exams[0].MainExamer, fmt.Sprintf("%v", exams[0].Groups), exams[0].ExamTypeFull})
		for _, exam := range exams[1:] {
			contents = append(contents, []string{"", strconv.Itoa(exam.AnCode), exam.MainExamer, fmt.Sprintf("%v", exam.Groups), exam.ExamTypeFull})
		}
		contents = append(contents, []string{"", "", "", "", ""})
	}
	return contents
}

// ConstraintsRows builds the constraints table rows (AnCode, Prüfender, Modul,
// Gruppe(n), Constraints), grouped and sorted by examer name; exams marked
// NotPlannedByMe are dropped. sameSlotExam resolves a same-slot partner ancode to its
// exam for the "zeitgleich: …" line (nil is tolerated only as far as the original code).
func ConstraintsRows(exams []*model.ZPAExamWithConstraints, sameSlotExam map[int]*model.ZPAExam) [][]string {
	byExamer := make(map[string][]*model.ZPAExamWithConstraints)
	for _, exam := range exams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		byExamer[exam.ZpaExam.MainExamer] = append(byExamer[exam.ZpaExam.MainExamer], exam)
	}
	keys := make([]string, 0, len(byExamer))
	for k := range byExamer {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	sorted := make([]*model.ZPAExamWithConstraints, 0, len(exams))
	for _, key := range keys {
		sorted = append(sorted, byExamer[key]...)
	}

	contents := make([][]string, 0)
	contents = append(contents, []string{"", "", "", ""})
	for _, exam := range sorted {
		if exam.Constraints == nil {
			continue
		}
		contents = append(contents, []string{
			strconv.Itoa(exam.ZpaExam.AnCode), exam.ZpaExam.MainExamer, exam.ZpaExam.Module,
			fmt.Sprintf("%v", exam.ZpaExam.Groups), "Constraints:",
		})
		c := exam.Constraints
		if c.NotPlannedByMe {
			contents = append(contents, []string{"", "", "", "", "- Termin wird von anderer Fakultät vorgegeben"})
		}
		if c.Online {
			contents = append(contents, []string{"", "", "", "", "- Fernprüfung gem. BayFEV "})
		}
		if len(c.ExcludeDays) > 0 {
			contents = append(contents, []string{"", "", "", "", fmt.Sprintf("- Nicht am %s", formatDays(c.ExcludeDays))})
		}
		if len(c.PossibleDays) > 0 {
			contents = append(contents, []string{"", "", "", "", fmt.Sprintf("- Möglich am %s", formatDays(c.PossibleDays))})
		}
		for _, sameSlotAncode := range c.SameSlot {
			other := sameSlotExam[sameSlotAncode]
			contents = append(contents, []string{"", "", "", "",
				fmt.Sprintf("- zeitgleich: %d. %s, %s, %v", sameSlotAncode, other.MainExamer, other.Module, other.Groups)})
		}
		if c.RoomConstraints != nil {
			if c.RoomConstraints.Seb {
				contents = append(contents, []string{"", "", "", "", "- SafeExamBrowser"})
			}
			if c.RoomConstraints.Exahm {
				contents = append(contents, []string{"", "", "", "", "- EXaHM"})
			}
			if c.RoomConstraints.Lab {
				contents = append(contents, []string{"", "", "", "", "- Labor"})
			}
			if c.RoomConstraints.PlacesWithSocket {
				contents = append(contents, []string{"", "", "", "", "- Steckdosen an den Sitzplätzen"})
			}
		}
		contents = append(contents, []string{"", "", "", ""})
	}
	return contents
}

// SameSlotAncodes returns the distinct same-slot partner ancodes referenced by the
// constraints, so the caller can pre-resolve their exams before rendering.
func SameSlotAncodes(exams []*model.ZPAExamWithConstraints) []int {
	seen := make(map[int]bool)
	out := make([]int, 0)
	for _, exam := range exams {
		if exam.Constraints == nil {
			continue
		}
		for _, a := range exam.Constraints.SameSlot {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	sort.Ints(out)
	return out
}

// formatDays renders a list of days as "02.01.06, 03.01.06".
func formatDays(days []*time.Time) string {
	s := ""
	for i, day := range days {
		if i == 0 {
			s = day.Format("02.01.06")
		} else {
			s = fmt.Sprintf("%s, %s", s, day.Format("02.01.06"))
		}
	}
	return s
}
