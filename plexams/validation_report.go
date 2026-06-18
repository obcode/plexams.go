package plexams

import (
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
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
// entities. Leave fields nil when they don't apply.
type ref struct {
	Ancode         *int
	RelatedAncodes []int
	Room           *string
	Day            *int
	Slot           *int
	InvigilatorID  *int
	StudentMtknr   *string
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
	v.findings = append(v.findings, &model.ValidationFinding{
		Level:          level,
		Message:        fmt.Sprintf(format, a...),
		Ancode:         r.Ancode,
		RelatedAncodes: r.RelatedAncodes,
		Room:           r.Room,
		Day:            r.Day,
		Slot:           r.Slot,
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

// finish renders the summary plus every finding to the reporter and returns the
// structured report. ok is true when there are no errors (warnings do not fail).
func (v *validation) finish() *model.ValidationReport {
	var errs, warns int
	for _, f := range v.findings {
		switch f.Level {
		case model.ValidationLevelError:
			errs++
		case model.ValidationLevelWarning:
			warns++
		}
	}

	if errs == 0 && warns == 0 {
		v.reporter.StopProgress(aurora.Sprintf(aurora.Green("✓ no problems found")))
	} else {
		v.reporter.StopProgressFail(aurora.Sprintf(aurora.Red("✗ %d error(s), %d warning(s)"), errs, warns))
		for _, f := range v.findings {
			v.reporter.Println(renderFinding(f))
		}
	}

	return &model.ValidationReport{
		Name:         v.name,
		Ok:           errs == 0,
		ErrorCount:   errs,
		WarningCount: warns,
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
