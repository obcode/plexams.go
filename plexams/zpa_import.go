package plexams

import (
	"context"
	"fmt"
	"reflect"

	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

// requirementsChanged reports whether the set of invigilator requirements
// differs (keyed by invigilator), so the todos only have to be rebuilt when the
// ZPA fetch actually changed something.
func requirementsChanged(oldReqs, newReqs []*zpa.SupervisorRequirements) bool {
	toMap := func(rs []*zpa.SupervisorRequirements) map[int]zpa.SupervisorRequirements {
		m := make(map[int]zpa.SupervisorRequirements, len(rs))
		for _, r := range rs {
			if r != nil {
				m[r.InvigilatorID] = *r
			}
		}
		return m
	}
	return !reflect.DeepEqual(toMap(oldReqs), toMap(newReqs))
}

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

	// if the requirements changed, rebuild the invigilator todos right away so
	// the cached snapshot reflects the new requirements. Best-effort.
	if requirementsChanged(oldReqs, reqs) {
		reporter.Step("requirements changed — rebuilding invigilator todos")
		if _, err := p.PrepareInvigilationTodos(ctx); err != nil {
			reporter.Warnf("cannot rebuild invigilator todos: %v", err)
		} else {
			reporter.StopProgress("invigilator todos rebuilt")
		}
	} else {
		reporter.Printf("no changes in requirements, invigilator todos unchanged")
	}

	p.markCondition(ctx, condInvigReqsImported)
	return len(reqs), nil
}
