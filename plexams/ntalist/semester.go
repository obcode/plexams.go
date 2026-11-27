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

// UntilEndOfStudies is stored as "valid until" for an NTA that holds until the
// end of the studies (semester column "*").
const UntilEndOfStudies = "Ende Studium"

// ParseValidity converts the semester column of the list into the "valid until"
// label of an NTA: "*" means until the end of the studies, "226:127" a range of
// semesters of which the last one is kept ("2027-SS"); a single code is that
// semester.
func ParseValidity(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return UntilEndOfStudies, nil
	}
	if raw == "" {
		return "", fmt.Errorf("Gültigkeit fehlt (Spalte semester)")
	}
	parts := strings.Split(raw, ":")
	last := strings.TrimSpace(parts[len(parts)-1])
	id, err := SemesterID(last)
	if err != nil {
		return "", fmt.Errorf("Gültigkeit „%s“ nicht erkannt (erwartet * oder z. B. 226:127)", raw)
	}
	return id, nil
}
