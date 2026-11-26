package plexams

import (
	"fmt"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
)

// Codes of findings the GUI offers an action for (ValidationFinding.code).
const (
	// FindingNTARoomNotAlone: an NTA entitled to a room of their own shares it. The
	// GUI offers to record a waiver.
	FindingNTARoomNotAlone = "NTA_ROOM_NOT_ALONE"
	// FindingNTARoomWaived: the same situation with a recorded waiver.
	FindingNTARoomWaived = "NTA_ROOM_WAIVED"
	// FindingPlanEntryNotAssembled: an exam has a time but no assembled exam.
	FindingPlanEntryNotAssembled = "PLAN_ENTRY_NOT_ASSEMBLED"
)

// ptr returns a pointer to v, for the optional reference fields of a finding.
func ptr[T any](v T) *T { return &v }

// validation accumulates the findings of a single validator and streams its
// progress and results through a Reporter. It replaces the old
// spinner + []string + fmt.Printf pattern: build findings with errorf/warnf/
// infof, then call finish to render the summary and return the structured
// report.
type validation struct {
	name     string
	reporter Reporter
	findings []*model.ValidationFinding
}

// ref carries the optional references that link a finding to the affected
// entities. Leave fields nil when they don't apply. Starttime is the finding's
// absolute exam start time (the sole time coordinate).
type ref struct {
	Ancode         *int
	RelatedAncodes []int
	Room           *string
	Starttime      *time.Time
	InvigilatorID  *int
	StudentMtknr   *string
	// Code is the stable kind of the finding, set where the GUI offers an action
	// for it. Hint tells the planner what to do about it.
	Code string
	Hint string
}

// hint returns r with the advice for the planner set: what to do about the finding.
func (r ref) hint(s string) ref {
	r.Hint = s
	return r
}

// newValidation starts a validator: name is the stable machine name (used in the
// report and by the GUI), title is the human status shown while it runs.
func newValidation(reporter Reporter, name, title string) *validation {
	reporter.Step(aurora.Sprintf(aurora.Cyan("%s"), title))
	return &validation{name: name, reporter: reporter}
}

// step updates the transient status line while the validator works.
func (v *validation) step(format string, a ...any) {
	v.reporter.Step(fmt.Sprintf(format, a...))
}

func (v *validation) add(level model.ValidationLevel, r ref, format string, a ...any) {
	var code, hint *string
	if r.Code != "" {
		code = &r.Code
	}
	if r.Hint != "" {
		hint = &r.Hint
	}
	v.findings = append(v.findings, &model.ValidationFinding{
		Code:           code,
		Hint:           hint,
		Level:          level,
		Message:        fmt.Sprintf(format, a...),
		Ancode:         r.Ancode,
		RelatedAncodes: r.RelatedAncodes,
		Room:           r.Room,
		Starttime:      r.Starttime,
		InvigilatorID:  r.InvigilatorID,
		StudentMtknr:   r.StudentMtknr,
	})
}

func (v *validation) errorf(r ref, format string, a ...any) {
	v.add(model.ValidationLevelError, r, format, a...)
}

func (v *validation) warnf(r ref, format string, a ...any) {
	v.add(model.ValidationLevelWarning, r, format, a...)
}

// infof records a purely informational finding (not a problem), e.g. an accepted
// exception. It does not count as a warning or error.
func (v *validation) infof(r ref, format string, a ...any) {
	v.add(model.ValidationLevelInfo, r, format, a...)
}

func (v *validation) counts() (errs, warns, infos int) {
	for _, f := range v.findings {
		switch f.Level {
		case model.ValidationLevelError:
			errs++
		case model.ValidationLevelWarning:
			warns++
		case model.ValidationLevelInfo:
			infos++
		}
	}
	return errs, warns, infos
}

// finish renders the summary plus every finding to the reporter and returns the
// structured report. ok is true when there are no errors (warnings and infos do
// not fail).
func (v *validation) finish() *model.ValidationReport {
	errs, warns, infos := v.counts()

	switch {
	case errs == 0 && warns == 0 && infos == 0:
		v.reporter.StopProgress(aurora.Sprintf(aurora.Green("✓ keine Probleme gefunden")))
	case errs == 0 && warns == 0:
		// only infos: not a problem, but show them.
		v.reporter.StopProgress(aurora.Sprintf(aurora.Green("✓ keine Probleme gefunden, %d Hinweis(e)"), infos))
		for _, f := range v.findings {
			v.reporter.Println(renderFinding(f))
		}
	default:
		v.reporter.StopProgressFail(aurora.Sprintf(aurora.Red("✗ %d Fehler, %d Warnung(en), %d Hinweis(e)"), errs, warns, infos))
		for _, f := range v.findings {
			v.reporter.Println(renderFinding(f))
		}
	}

	return &model.ValidationReport{
		Name:         v.name,
		Ok:           errs == 0,
		ErrorCount:   errs,
		WarningCount: warns,
		InfoCount:    infos,
		Findings:     v.findings,
	}
}

// skip marks the validation as not applicable because the data it checks does not
// exist yet (e.g. no exam plan, no planned rooms, no invigilations). It is not a
// failure: the report is ok with no findings, flagged Skipped so the GUI can render a
// neutral "übersprungen" state instead of a green pass.
func (v *validation) skip(reason string) *model.ValidationReport {
	v.reporter.StopProgress(aurora.Sprintf(aurora.Gray(14, "– übersprungen: %s"), reason))
	return &model.ValidationReport{
		Name:       v.name,
		Ok:         true,
		Skipped:    true,
		SkipReason: &reason,
		Findings:   []*model.ValidationFinding{},
	}
}

// report returns the structured report from the accumulated findings without
// streaming a summary or the findings. Use it when a validator does its own
// custom terminal output instead of the flat list finish produces.
func (v *validation) report() *model.ValidationReport {
	errs, warns, infos := v.counts()
	return &model.ValidationReport{
		Name:         v.name,
		Ok:           errs == 0,
		ErrorCount:   errs,
		WarningCount: warns,
		InfoCount:    infos,
		Findings:     v.findings,
	}
}

// renderFinding formats a finding as a colored terminal line.
func renderFinding(f *model.ValidationFinding) string {
	switch f.Level {
	case model.ValidationLevelError:
		return aurora.Sprintf(aurora.Red("  ✗ %s"), f.Message)
	case model.ValidationLevelWarning:
		return aurora.Sprintf(aurora.Yellow("  ⚠ %s"), f.Message)
	default:
		return aurora.Sprintf(aurora.Gray(14, "  • %s"), f.Message)
	}
}
