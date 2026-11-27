// Package ntalist parses the NTA lists (Nachteilsausgleiche) that the exam board
// publishes per faculty as Excel files in a SharePoint folder. The lists are a
// PRIMUSS export; see .claude/memory/nta-list-import.md for the format.
//
// The package is pure: it reads bytes and returns what it found. Matching the
// rows against the NTA master data happens in package plexams.
package ntalist

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// semesterCodeRE matches the three-digit PRIMUSS semester code: the first digit
// is the term (1 = summer, 2 = winter), the other two the year, e.g. 226 for the
// winter term 2026/27 and 127 for the summer term 2027.
var semesterCodeRE = regexp.MustCompile(`^([12])(\d{2})$`)

// SemesterID converts a PRIMUSS semester code ("226") into a plexams semester id
// ("2026-WS").
func SemesterID(code string) (string, error) {
	m := semesterCodeRE.FindStringSubmatch(strings.TrimSpace(code))
	if m == nil {
		return "", fmt.Errorf("Semester „%s“ nicht erkannt (erwartet z. B. 226 für WS 26/27)", code)
	}
	year, _ := strconv.Atoi(m[2])
	term := "SS"
	if m[1] == "2" {
		term = "WS"
	}
	return fmt.Sprintf("%d-%s", 2000+year, term), nil
}

// Validity is the semester column of the list: "*" means the NTA holds for the
// whole studies, "226:127" a range of semesters of which only the last one
// matters.
type Validity struct {
	WholeStudies bool `json:"wholeStudies"`
	// LastSemester is the semester id of the last semester ("2027-SS"); empty for
	// WholeStudies.
	LastSemester string `json:"lastSemester"`
}

// ParseValidity reads the semester column ("*", "226:127", "226").
func ParseValidity(raw string) (Validity, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return Validity{WholeStudies: true}, nil
	}
	if raw == "" {
		return Validity{}, fmt.Errorf("Gültigkeit fehlt (Spalte semester)")
	}
	parts := strings.Split(raw, ":")
	id, err := SemesterID(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil {
		return Validity{}, fmt.Errorf("Gültigkeit „%s“ nicht erkannt (erwartet * oder z. B. 226:127)", raw)
	}
	return Validity{LastSemester: id}, nil
}

// Label is the "valid until" text of the NTA master data, in the convention the
// stored NTAs use: "Studium IF" for the whole studies in program IF,
// "Wintersemester 2026/2027" or "Sommersemester 2027" for a last semester.
func (v Validity) Label(program string) string {
	if v.WholeStudies {
		return strings.TrimSpace("Studium " + program)
	}
	year, term, ok := strings.Cut(v.LastSemester, "-")
	if !ok {
		return v.LastSemester
	}
	if term == "WS" {
		y, err := strconv.Atoi(year)
		if err != nil {
			return v.LastSemester
		}
		return fmt.Sprintf("Wintersemester %d/%d", y, y+1)
	}
	return "Sommersemester " + year
}
