package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// InvigilatorConstraints returns all per-invigilator constraints stored in the DB.
func (p *Plexams) InvigilatorConstraints(ctx context.Context) ([]*model.InvigilatorConstraints, error) {
	return p.dbClient.InvigilatorConstraints(ctx)
}

// invigilatorConstraintsMap loads all per-invigilator constraints into a map
// keyed by teacher ID (so the invigilator loops do not query per teacher).
func (p *Plexams) invigilatorConstraintsMap(ctx context.Context) (map[int]*model.InvigilatorConstraints, error) {
	constraints, err := p.dbClient.InvigilatorConstraints(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[int]*model.InvigilatorConstraints, len(constraints))
	for _, c := range constraints {
		m[c.TeacherID] = c
	}
	return m, nil
}

// rebuildInvigilationTodosBestEffort recomputes the cached invigilator todos
// after a change to the invigilator pool (constraints / permanent list). It is
// best-effort: the constraint change itself already succeeded, so a failure here
// (e.g. no room plan yet) is only logged.
func (p *Plexams) rebuildInvigilationTodosBestEffort(ctx context.Context) {
	if _, err := p.PrepareInvigilationTodos(ctx); err != nil {
		log.Error().Err(err).Msg("cannot rebuild invigilation todos after invigilator-pool change")
	}
}

// PermanentNonInvigilators returns the teachers who never do invigilation duty
// again (global, carries over between semesters).
func (p *Plexams) PermanentNonInvigilators(ctx context.Context) ([]*model.PermanentNonInvigilator, error) {
	return p.dbClient.PermanentNonInvigilators(ctx)
}

// InvigilatorCandidates returns all teachers in the invigilator pool, including
// those currently excluded (isNotInvigilator / permanent). The GUI uses this to
// manage constraints for everyone, since invigilatorsWithReq drops the excluded
// ones.
func (p *Plexams) InvigilatorCandidates(ctx context.Context) ([]*model.Teacher, error) {
	return p.getInvigilators(ctx)
}

// SetPermanentNonInvigilator adds or updates a permanent (cross-semester)
// non-invigilator. If name is empty it tries to resolve the teacher's name (so a
// CLI/API caller need not supply it for teachers still in the pool).
func (p *Plexams) SetPermanentNonInvigilator(ctx context.Context, teacherID int, name, reason string) (*model.PermanentNonInvigilator, error) {
	if name == "" {
		if teacher, err := p.dbClient.GetTeacher(ctx, teacherID); err == nil && teacher != nil {
			name = teacher.Fullname
		}
	}
	nonInvigilator := &model.PermanentNonInvigilator{TeacherID: teacherID, Name: name, Reason: reason}
	if err := p.dbClient.UpsertPermanentNonInvigilator(ctx, nonInvigilator); err != nil {
		return nil, err
	}
	p.rebuildInvigilationTodosBestEffort(ctx)
	return nonInvigilator, nil
}

// RemovePermanentNonInvigilator removes a permanent non-invigilator.
func (p *Plexams) RemovePermanentNonInvigilator(ctx context.Context, teacherID int) (bool, error) {
	removed, err := p.dbClient.DeletePermanentNonInvigilator(ctx, teacherID)
	if err == nil {
		p.rebuildInvigilationTodosBestEffort(ctx)
	}
	return removed, err
}

// notInvigilating builds a predicate that reports whether a teacher does no
// invigilation at all — true if they are a global permanent non-invigilator or
// their per-semester constraint has isNotInvigilator set. It also returns the
// per-semester constraints map so callers can build the invigilators without a
// second query.
func (p *Plexams) notInvigilating(ctx context.Context) (func(teacherID int) bool, map[int]*model.InvigilatorConstraints, error) {
	cmap, err := p.invigilatorConstraintsMap(ctx)
	if err != nil {
		return nil, nil, err
	}
	permanent, err := p.dbClient.PermanentNonInvigilators(ctx)
	if err != nil {
		return nil, nil, err
	}
	permanentSet := make(map[int]bool, len(permanent))
	for _, n := range permanent {
		permanentSet[n.TeacherID] = true
	}
	isNot := func(teacherID int) bool {
		if permanentSet[teacherID] {
			return true
		}
		c := cmap[teacherID]
		return c != nil && c.IsNotInvigilator
	}
	return isNot, cmap, nil
}

// SetInvigilatorConstraints creates or replaces the whole constraints record of
// one invigilator. Empty records (no exclusion, no dates, no windows) are stored
// too, so the GUI can keep an explicit "no constraints" entry.
func (p *Plexams) SetInvigilatorConstraints(ctx context.Context, input model.InvigilatorConstraintsInput) (*model.InvigilatorConstraints, error) {
	timeWindows := make([]*model.InvigilationTimeWindow, 0, len(input.TimeWindows))
	for _, w := range input.TimeWindows {
		if w == nil {
			continue
		}
		if w.From == nil && w.Until == nil {
			return nil, fmt.Errorf("time window on %s: at least one of from/until must be set", w.Date.Format("02.01.2006"))
		}
		if w.From != nil && w.Until != nil && !w.Until.After(*w.From) {
			return nil, fmt.Errorf("time window on %s: until must be after from", w.Date.Format("02.01.2006"))
		}
		timeWindows = append(timeWindows, &model.InvigilationTimeWindow{Date: w.Date, From: w.From, Until: w.Until})
	}

	excludedDates := make([]time.Time, 0, len(input.ExcludedDates))
	for _, d := range input.ExcludedDates {
		if d != nil {
			excludedDates = append(excludedDates, *d)
		}
	}

	constraints := &model.InvigilatorConstraints{
		TeacherID:        input.TeacherID,
		IsNotInvigilator: input.IsNotInvigilator,
		ExcludedDates:    excludedDates,
		TimeWindows:      timeWindows,
	}
	if err := p.dbClient.UpsertInvigilatorConstraints(ctx, constraints); err != nil {
		return nil, err
	}
	p.rebuildInvigilationTodosBestEffort(ctx)
	return constraints, nil
}

// DeleteInvigilatorConstraints removes the constraints record of one invigilator.
func (p *Plexams) DeleteInvigilatorConstraints(ctx context.Context, teacherID int) (bool, error) {
	removed, err := p.dbClient.DeleteInvigilatorConstraints(ctx, teacherID)
	if err == nil {
		p.rebuildInvigilationTodosBestEffort(ctx)
	}
	return removed, err
}
