package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/zpaimport"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

// reportImportChanges diffs a ZPA (re-)import against the previous DB state via
// zpaimport.DiffChanges, streams the resulting report lines through the reporter and
// returns the partial SyncLogEntry (Added/Changed/Removed/Entries) that the caller
// completes (operation/label/…) and logs.
func reportImportChanges[T any](reporter Reporter, old, neu []T,
	id func(T) int, name func(T) string, fields func(T) map[string]string) *model.SyncLogEntry {
	rec, msgs := zpaimport.DiffChanges(old, neu, id, name, fields)
	for _, m := range msgs {
		reporter.Printf("%s", m)
	}
	return rec
}

// SyncLog returns the transfer history (newest first; limit <= 0 = all).
func (p *Plexams) SyncLog(ctx context.Context, limit int) ([]*model.SyncLogEntry, error) {
	return p.dbClient.SyncLog(ctx, limit)
}

// logSync stamps the entry with the current time and appends it to the sync-log
// history (best-effort).
func (p *Plexams) logSync(ctx context.Context, entry *model.SyncLogEntry) {
	entry.Time = time.Now()
	if err := p.dbClient.AddSyncLogEntry(ctx, entry); err != nil {
		log.Error().Err(err).Str("operation", entry.Operation).Msg("cannot write sync-log entry")
	}
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
	rec := reportImportChanges(reporter, oldTeachers, teachers,
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
	rec.Operation = "zpa-import-teachers"
	rec.Label = "Lehrende aus ZPA importiert"
	rec.Direction = "import"
	rec.System = "ZPA"
	rec.OK = true
	rec.Summary = fmt.Sprintf("%d Lehrende geladen (%d neu, %d geändert, %d entfallen)",
		len(teachers), rec.Added, rec.Changed, rec.Removed)
	p.logSync(ctx, rec)
	p.markCondition(ctx, condZPAPersonsImported)
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
	rec := reportImportChanges(reporter, oldExams, exams,
		func(e *model.ZPAExam) int { return e.AnCode },
		func(e *model.ZPAExam) string { return fmt.Sprintf("%d. %s (%s)", e.AnCode, e.Module, e.MainExamer) },
		func(e *model.ZPAExam) map[string]string {
			groups := append([]string{}, e.Groups...)
			sort.Strings(groups)
			// NB: primussAncodes are intentionally not compared here — they are
			// enriched on DB read (cleanupPrimussAncodes + added ancodes) while the
			// ZPA fetch returns the raw list, so they would always differ.
			return map[string]string{
				"module":   e.Module,
				"examer":   fmt.Sprintf("%s (%d)", e.MainExamer, e.MainExamerID),
				"type":     e.ExamType,
				"duration": fmt.Sprint(e.Duration),
				"repeater": fmt.Sprint(e.IsRepeaterExam),
				"groups":   strings.Join(groups, ","),
			}
		})
	rec.Operation = "zpa-import-exams"
	rec.Label = "Prüfungen aus ZPA importiert"
	rec.Direction = "import"
	rec.System = "ZPA"
	rec.OK = true
	rec.Summary = fmt.Sprintf("%d Prüfungen geladen (%d neu, %d geändert, %d entfallen)",
		len(exams), rec.Added, rec.Changed, rec.Removed)
	p.logSync(ctx, rec)
	p.markCondition(ctx, condZPAImported)
	reporter.StopProgress(fmt.Sprintf("fetched %d exams", len(exams)))

	// pre-select the planning status of exams that have no decision yet: written and
	// practical exams ("schriftliche/praktische Prüfung") are to be planned, everything
	// else is not. Manual decisions are preserved.
	if toPlan, notToPlan, err := p.autoPreselectExamsToPlan(ctx); err != nil {
		log.Error().Err(err).Msg("cannot pre-select exams to plan")
	} else if toPlan+notToPlan > 0 {
		reporter.Step(fmt.Sprintf("Vorauswahl: %d Prüfungen zu planen, %d nicht planen (nur bisher unentschiedene)", toPlan, notToPlan))
	}

	return len(exams), nil
}

// autoPreselectExamsToPlan sets the planning status of all exams that have none yet
// (written/practical → to plan, rest → not to plan) while keeping every existing
// manual decision. Returns how many were newly set to-plan / not-to-plan.
func (p *Plexams) autoPreselectExamsToPlan(ctx context.Context) (toPlanAdded, notToPlanAdded int, err error) {
	f := false
	all, err := p.GetZPAExams(ctx, &f)
	if err != nil {
		return 0, 0, err
	}
	toPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
	if err != nil {
		return 0, 0, err
	}
	notToPlan, err := p.dbClient.GetZPAExamsNotToPlan(ctx)
	if err != nil {
		return 0, 0, err
	}

	newToPlan, newNotToPlan, toPlanAdded, notToPlanAdded := zpaimport.Preselect(all, toPlan, notToPlan)
	if toPlanAdded+notToPlanAdded == 0 {
		return 0, 0, nil // nothing undecided
	}
	if err := p.dbClient.SetZPAExamsToPlan(ctx, newToPlan, newNotToPlan); err != nil {
		return 0, 0, err
	}
	return toPlanAdded, notToPlanAdded, nil
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
	rec := reportImportChanges(reporter, oldReqs, reqs,
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
	rec.Operation = "zpa-import-invigilator-requirements"
	rec.Label = "Aufsichts-Anforderungen aus ZPA importiert"
	rec.Direction = "import"
	rec.System = "ZPA"
	rec.OK = true
	rec.Summary = fmt.Sprintf("%d Anforderungen geladen (%d neu, %d geändert, %d entfallen), %d fehlen noch",
		len(reqs), rec.Added, rec.Changed, rec.Removed, missing)
	p.logSync(ctx, rec)
	if rec.Added+rec.Changed+rec.Removed > 0 {
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
