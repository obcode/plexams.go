package plexams

import (
	"context"
	"fmt"
)

// ImportTeachersFromZPA fetches the teachers from ZPA, caches them and streams a
// CLI-style listing via the reporter. It returns the number fetched.
func (p *Plexams) ImportTeachersFromZPA(ctx context.Context, reporter Reporter) (int, error) {
	reporter.Step("fetching teachers from ZPA")
	fromZPA := true
	teachers, err := p.GetTeachers(ctx, &fromZPA)
	if err != nil {
		return 0, err
	}
	for i, teacher := range teachers {
		reporter.Printf("%3d. %s", i+1, teacher.Fullname)
	}
	reporter.StopProgress(fmt.Sprintf("fetched %d teachers", len(teachers)))
	return len(teachers), nil
}

// ImportExamsFromZPA fetches the exams from ZPA, caches them and streams a
// CLI-style listing via the reporter. It returns the number fetched.
func (p *Plexams) ImportExamsFromZPA(ctx context.Context, reporter Reporter) (int, error) {
	reporter.Step("fetching exams from ZPA")
	fromZPA := true
	exams, err := p.GetZPAExams(ctx, &fromZPA)
	if err != nil {
		return 0, err
	}
	for _, exam := range exams {
		reporter.Printf("%3d. %s (%s): %v", exam.AnCode, exam.Module, exam.MainExamer, exam.PrimussAncodes)
	}
	reporter.StopProgress(fmt.Sprintf("fetched %d exams", len(exams)))
	return len(exams), nil
}

// ImportInvigilatorRequirementsFromZPA fetches the invigilator requirements from
// ZPA, streams which invigilators are still missing and returns the number of
// requirements fetched.
func (p *Plexams) ImportInvigilatorRequirementsFromZPA(ctx context.Context, reporter Reporter) (int, error) {
	reporter.Step("fetching invigilator requirements from ZPA")
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
	return len(reqs), nil
}
