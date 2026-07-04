// Package repeatcalc holds the pure study-group-semester repeat heuristic: reading the
// semester number out of a study-group code and deciding whether an exam is (likely) a
// repeat for a student. It drives the conflict auto-accept (a repeat conflict is only
// informational) and the plan-generation down-weighting. All functions are I/O-free;
// the heuristic is not fully reliable (study-group numbers are the only signal), so it
// only down-weights, never hard-excludes.
package repeatcalc

import "strings"

// SemesterOf extracts the first run of digits from a study-group code (e.g. "IF4B"
// -> 4), or 0 if none.
func SemesterOf(group string) int {
	start := strings.IndexFunc(group, func(r rune) bool { return r >= '0' && r <= '9' })
	if start < 0 {
		return 0
	}
	end := start
	for end < len(group) && group[end] >= '0' && group[end] <= '9' {
		end++
	}
	n := 0
	for _, c := range group[start:end] {
		n = n*10 + int(c-'0')
	}
	return n
}

// MinGroupSemester returns the smallest semester number found in the exam's groups
// (e.g. "IF2A" -> 2), or 0 if none.
func MinGroupSemester(groups []string) int {
	min := 0
	for _, g := range groups {
		if s := SemesterOf(g); s > 0 && (min == 0 || s < min) {
			min = s
		}
	}
	return min
}

// RepeatForStudent reports whether an exam is (likely) a repeat for a student: the
// exam is flagged a repeater, or the student's semester is higher than the exam's
// (a heuristic via study-group numbers; not fully reliable).
func RepeatForStudent(studentSemester int, examRepeater bool, examSemester int) bool {
	if examRepeater {
		return true
	}
	return studentSemester > 0 && examSemester > 0 && studentSemester > examSemester
}
