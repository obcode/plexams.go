// Package zpaimport holds the pure decision logic of the ZPA import: diffing a fresh
// import against the previous DB state into a SyncLogEntry plus human-readable report
// lines, classifying a ZPA exam for the automatic to-plan pre-selection, and computing
// the new to-plan / not-to-plan sets. All functions are I/O-free over graph/model
// types; the ZPA client fetches, DB writes, reporter output and condition marking stay
// in the plexams package.
package zpaimport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// DiffChanges compares a fresh ZPA import (neu) against the previous DB state (old),
// keyed by id, and returns a partial SyncLogEntry (Added/Changed/Removed/Entries) that
// the caller completes (operation/label/…) together with the human-readable report
// lines, in order, for the caller to emit. New entries, per-field changes on existing
// entries and dropped entries are reported.
func DiffChanges[T any](old, neu []T,
	id func(T) int, name func(T) string, fields func(T) map[string]string) (*model.SyncLogEntry, []string) {
	oldByID := make(map[int]T, len(old))
	for _, o := range old {
		oldByID[id(o)] = o
	}
	newByID := make(map[int]T, len(neu))
	for _, n := range neu {
		newByID[id(n)] = n
	}

	rec := &model.SyncLogEntry{Entries: make([]*model.SyncChangeEntry, 0)}
	msgs := make([]string, 0)

	newIDs := make([]int, 0, len(newByID))
	for k := range newByID {
		newIDs = append(newIDs, k)
	}
	sort.Ints(newIDs)
	for _, k := range newIDs {
		n := newByID[k]
		o, ok := oldByID[k]
		if !ok {
			msgs = append(msgs, fmt.Sprintf("  + neu: %s", name(n)))
			rec.Entries = append(rec.Entries, &model.SyncChangeEntry{Type: "added", Name: name(n)})
			rec.Added++
			continue
		}
		of, nf := fields(o), fields(n)
		fnames := make([]string, 0, len(nf))
		for f := range nf {
			fnames = append(fnames, f)
		}
		sort.Strings(fnames)
		diffs := make([]string, 0)
		fieldChanges := make([]*model.SyncFieldChange, 0)
		for _, f := range fnames {
			if of[f] != nf[f] {
				diffs = append(diffs, fmt.Sprintf("%s: %q → %q", f, of[f], nf[f]))
				fieldChanges = append(fieldChanges, &model.SyncFieldChange{Field: f, Old: of[f], New: nf[f]})
			}
		}
		if len(diffs) > 0 {
			msgs = append(msgs, fmt.Sprintf("  ~ %s: %s", name(n), strings.Join(diffs, ", ")))
			rec.Entries = append(rec.Entries, &model.SyncChangeEntry{Type: "changed", Name: name(n), Fields: fieldChanges})
			rec.Changed++
		}
	}

	oldIDs := make([]int, 0, len(oldByID))
	for k := range oldByID {
		oldIDs = append(oldIDs, k)
	}
	sort.Ints(oldIDs)
	for _, k := range oldIDs {
		if _, ok := newByID[k]; !ok {
			msgs = append(msgs, fmt.Sprintf("  - entfällt: %s", name(oldByID[k])))
			rec.Entries = append(rec.Entries, &model.SyncChangeEntry{Type: "removed", Name: name(oldByID[k])})
			rec.Removed++
		}
	}

	if rec.Added == 0 && rec.Removed == 0 && rec.Changed == 0 {
		msgs = append(msgs, "keine Änderungen gegenüber dem vorherigen Stand")
	} else {
		msgs = append(msgs, fmt.Sprintf("Änderungen: %d neu, %d geändert, %d entfallen", rec.Added, rec.Changed, rec.Removed))
	}
	return rec, msgs
}

// ExamShouldBePlanned classifies a ZPA exam for the automatic pre-selection: written
// and practical exams ("schriftliche/praktische Prüfung") are planned centrally, all
// other types (Modularbeit, Präsentation, mündliche Prüfung, Schein, extern, …) are not.
func ExamShouldBePlanned(e *model.ZPAExam) bool {
	t := strings.ToLower(e.ExamTypeFull)
	return strings.Contains(t, "schriftliche prüfung") || strings.Contains(t, "praktische prüfung")
}

// Preselect computes the new to-plan / not-to-plan sets for all exams that have no
// decision yet (written/practical → to plan, rest → not to plan) while keeping every
// existing decision in toPlan/notToPlan. It returns the extended sets and how many were
// newly added to each. When nothing was undecided both added counts are 0 and the
// returned sets equal the inputs.
func Preselect(all, toPlan, notToPlan []*model.ZPAExam) (newToPlan, newNotToPlan []*model.ZPAExam, toPlanAdded, notToPlanAdded int) {
	decided := make(map[int]bool, len(toPlan)+len(notToPlan))
	for _, e := range toPlan {
		decided[e.AnCode] = true
	}
	for _, e := range notToPlan {
		decided[e.AnCode] = true
	}

	newToPlan = append([]*model.ZPAExam{}, toPlan...)
	newNotToPlan = append([]*model.ZPAExam{}, notToPlan...)
	for _, e := range all {
		if decided[e.AnCode] {
			continue
		}
		if ExamShouldBePlanned(e) {
			newToPlan = append(newToPlan, e)
			toPlanAdded++
		} else {
			newNotToPlan = append(newNotToPlan, e)
			notToPlanAdded++
		}
	}
	return newToPlan, newNotToPlan, toPlanAdded, notToPlanAdded
}
