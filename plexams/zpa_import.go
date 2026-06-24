package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

// reportImportChanges streams what a ZPA (re-)import changed compared to the
// previous DB state: new entries, dropped entries, and per-field changes of
// existing ones (keyed by id). It returns whether anything changed at all.
func reportImportChanges[T any](reporter Reporter, old, neu []T,
	id func(T) int, name func(T) string, fields func(T) map[string]string) bool {
	oldByID := make(map[int]T, len(old))
	for _, o := range old {
		oldByID[id(o)] = o
	}
	newByID := make(map[int]T, len(neu))
	for _, n := range neu {
		newByID[id(n)] = n
	}

	added, removed, changed := 0, 0, 0

	newIDs := make([]int, 0, len(newByID))
	for k := range newByID {
		newIDs = append(newIDs, k)
	}
	sort.Ints(newIDs)
	for _, k := range newIDs {
		n := newByID[k]
		o, ok := oldByID[k]
		if !ok {
			reporter.Printf("  + neu: %s", name(n))
			added++
			continue
		}
		of, nf := fields(o), fields(n)
		fnames := make([]string, 0, len(nf))
		for f := range nf {
			fnames = append(fnames, f)
		}
		sort.Strings(fnames)
		diffs := make([]string, 0)
		for _, f := range fnames {
			if of[f] != nf[f] {
				diffs = append(diffs, fmt.Sprintf("%s: %q → %q", f, of[f], nf[f]))
			}
		}
		if len(diffs) > 0 {
			reporter.Printf("  ~ %s: %s", name(n), strings.Join(diffs, ", "))
			changed++
		}
	}

	oldIDs := make([]int, 0, len(oldByID))
	for k := range oldByID {
		oldIDs = append(oldIDs, k)
	}
	sort.Ints(oldIDs)
	for _, k := range oldIDs {
		if _, ok := newByID[k]; !ok {
			reporter.Printf("  - entfällt: %s", name(oldByID[k]))
			removed++
		}
	}

	if added == 0 && removed == 0 && changed == 0 {
		reporter.Printf("keine Änderungen gegenüber dem vorherigen Stand")
		return false
	}
	reporter.Printf("Änderungen: %d neu, %d geändert, %d entfallen", added, changed, removed)
	return true
}

// ImportTeachersFromZPA fetches the teachers from ZPA, caches them and streams a
// CLI-style listing via the reporter. It returns the number fetched.
func (p *Plexams) ImportTeachersFromZPA(ctx context.Context, reporter Reporter) (int, error) {
	reporter.Step("fetching teachers from ZPA")
	falseVal := false
	oldTeachers, err := p.GetTeachers(ctx, &falseVal) // current DB state, before the fetch overwrites it
	if err != nil {
		log.Error().Err(err).Msg("cannot read current teachers")
	}
	fromZPA := true
	teachers, err := p.GetTeachers(ctx, &fromZPA)
	if err != nil {
		return 0, err
	}
	reportImportChanges(reporter, oldTeachers, teachers,
		func(t *model.Teacher) int { return t.ID },
		func(t *model.Teacher) string { return fmt.Sprintf("%s (%d)", t.Fullname, t.ID) },
		func(t *model.Teacher) map[string]string {
			return map[string]string{
				"name":     t.Fullname,
				"email":    t.Email,
				"fk":       t.FK,
				"isProf":   fmt.Sprint(t.IsProf),
				"isProfHC": fmt.Sprint(t.IsProfHC),
				"isLBA":    fmt.Sprint(t.IsLBA),
				"isActive": fmt.Sprint(t.IsActive),
			}
		})
	reporter.StopProgress(fmt.Sprintf("fetched %d teachers", len(teachers)))
	return len(teachers), nil
}

// ImportExamsFromZPA fetches the exams from ZPA, caches them and streams a
// CLI-style listing via the reporter. It returns the number fetched.
func (p *Plexams) ImportExamsFromZPA(ctx context.Context, reporter Reporter) (int, error) {
	reporter.Step("fetching exams from ZPA")
	falseVal := false
	oldExams, err := p.GetZPAExams(ctx, &falseVal) // current DB state, before the fetch overwrites it
	if err != nil {
		log.Error().Err(err).Msg("cannot read current exams")
	}
	fromZPA := true
	exams, err := p.GetZPAExams(ctx, &fromZPA)
	if err != nil {
		return 0, err
	}
	reportImportChanges(reporter, oldExams, exams,
		func(e *model.ZPAExam) int { return e.AnCode },
		func(e *model.ZPAExam) string { return fmt.Sprintf("%d. %s (%s)", e.AnCode, e.Module, e.MainExamer) },
		func(e *model.ZPAExam) map[string]string {
			groups := append([]string{}, e.Groups...)
			sort.Strings(groups)
			return map[string]string{
				"module":     e.Module,
				"examer":     fmt.Sprintf("%s (%d)", e.MainExamer, e.MainExamerID),
				"type":       e.ExamType,
				"duration":   fmt.Sprint(e.Duration),
				"repeater":   fmt.Sprint(e.IsRepeaterExam),
				"groups":     strings.Join(groups, ","),
				"primussAnc": fmt.Sprintf("%v", e.PrimussAncodes),
			}
		})
	p.markCondition(ctx, condZPAImported)
	reporter.StopProgress(fmt.Sprintf("fetched %d exams", len(exams)))
	return len(exams), nil
}

// ImportInvigilatorRequirementsFromZPA fetches the invigilator requirements from
// ZPA, streams which invigilators are still missing and returns the number of
// requirements fetched.
func (p *Plexams) ImportInvigilatorRequirementsFromZPA(ctx context.Context, reporter Reporter) (int, error) {
	reporter.Step("fetching invigilator requirements from ZPA")

	// snapshot the stored requirements before the fetch overwrites them, so we
	// can tell whether anything actually changed.
	oldReqs, err := p.dbClient.AllInvigilatorRequirements(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read current invigilator requirements")
	}

	reqs, err := p.GetSupervisorRequirements(ctx)
	if err != nil {
		return 0, err
	}
	reporter.Printf("fetched %d invigilator requirements", len(reqs))

	invigilators, err := p.GetInvigilators(ctx)
	if err != nil {
		return 0, err
	}

	missing := 0
	for _, invigilator := range invigilators {
		if !invigilator.HasSubmittedRequirements {
			missing++
			reporter.Printf("  - missing: %s (%s)", invigilator.Teacher.Fullname, invigilator.Teacher.Email)
		}
	}
	if missing == 0 {
		reporter.StopProgress(fmt.Sprintf("%d requirements fetched, all invigilators have submitted", len(reqs)))
	} else {
		reporter.StopProgressFail(fmt.Sprintf("%d requirements fetched, %d invigilator(s) still missing", len(reqs), missing))
	}

	// report what changed; if anything did, rebuild the invigilator todos right
	// away so the cached snapshot reflects the new requirements (best-effort).
	changed := reportImportChanges(reporter, oldReqs, reqs,
		func(r *zpa.SupervisorRequirements) int { return r.InvigilatorID },
		func(r *zpa.SupervisorRequirements) string {
			return fmt.Sprintf("%s (%d)", r.Invigilator, r.InvigilatorID)
		},
		func(r *zpa.SupervisorRequirements) map[string]string {
			excl := append([]string{}, r.ExcludedDates...)
			sort.Strings(excl)
			return map[string]string{
				"partTime":      fmt.Sprint(r.PartTime),
				"oralExams":     fmt.Sprint(r.OralExamsContribution),
				"livecoding":    fmt.Sprint(r.LivecodingContribution),
				"master":        fmt.Sprint(r.MasterContribution),
				"freeSemester":  fmt.Sprint(r.FreeSemester),
				"overtimeLast":  fmt.Sprint(r.OvertimeLastSemester),
				"overtimeThis":  fmt.Sprint(r.OvertimeThisSemester),
				"excludedDates": strings.Join(excl, ","),
			}
		})
	if changed {
		reporter.Step("requirements changed — rebuilding invigilator todos")
		if _, err := p.PrepareInvigilationTodos(ctx); err != nil {
			reporter.Warnf("cannot rebuild invigilator todos: %v", err)
		} else {
			reporter.StopProgress("invigilator todos rebuilt")
		}
	}

	p.markCondition(ctx, condInvigReqsImported)
	return len(reqs), nil
}
