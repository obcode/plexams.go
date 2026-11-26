package plexams

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/rs/zerolog/log"
)

var semesterYearRE = regexp.MustCompile(`\d{4}`)

// semesterOrdinal maps a semester label into a monotonically increasing integer
// so semesters can be range-compared. It accepts the runtime form ("2026 SS"),
// the create-form ("2026-SS") and "2026SS"; the summer semester (SS) sorts
// before the winter semester (WS) of the same year (WS starts the academic
// year, e.g. "2025 WS" < "2026 SS" < "2026 WS"). ok is false when the label is
// not a recognizable semester -- it also reads stored labels (nta.last_semester),
// which go back further than today's format.
func semesterOrdinal(label string) (int, bool) {
	s := strings.ToUpper(strings.TrimSpace(label))
	var season int
	switch {
	case strings.HasSuffix(s, "SS"):
		season = 0
	case strings.HasSuffix(s, "WS"):
		season = 1
	default:
		return 0, false
	}
	yearStr := semesterYearRE.FindString(s)
	if yearStr == "" {
		return 0, false
	}
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return 0, false
	}
	return year*2 + season, true
}

// permanentAppliesTo reports whether a permanent non-invigilator entry applies
// to the semester with ordinal curOrd. It errs toward keeping the exclusion: if
// the current semester or a bound cannot be parsed the corresponding check is
// skipped rather than treated as "does not apply".
func permanentAppliesTo(n *model.PermanentNonInvigilator, curOrd int, curOK bool) bool {
	if !curOK {
		return true
	}
	if n.ValidFrom != nil {
		if fromOrd, ok := semesterOrdinal(*n.ValidFrom); ok && curOrd < fromOrd {
			return false
		}
	}
	if n.ValidUntil != nil {
		if untilOrd, ok := semesterOrdinal(*n.ValidUntil); ok && curOrd > untilOrd {
			return false
		}
	}
	return true
}

// normalizeSemesterBound trims a validity bound and maps empty/blank to nil (an
// open-ended bound).
func normalizeSemesterBound(bound *string) *string {
	if bound == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*bound)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

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
// CLI/API caller need not supply it for teachers still in the pool). validFrom
// and validUntil (semester labels like "2026-SS", either may be nil/empty for an
// open bound) limit the semesters the exemption applies to.
func (p *Plexams) SetPermanentNonInvigilator(ctx context.Context, teacherID int, name, reason string, validFrom, validUntil *string) (*model.PermanentNonInvigilator, error) {
	validFrom = normalizeSemesterBound(validFrom)
	validUntil = normalizeSemesterBound(validUntil)

	var fromOrd, untilOrd int
	if validFrom != nil {
		ord, ok := semesterOrdinal(*validFrom)
		if !ok {
			return nil, apierr.Newf(apierr.CodeInvalidInput, "„%s“ ist kein Semester. Gib den Beginn z. B. als 2026-SS oder 2026-WS an.", *validFrom)
		}
		fromOrd = ord
	}
	if validUntil != nil {
		ord, ok := semesterOrdinal(*validUntil)
		if !ok {
			return nil, apierr.Newf(apierr.CodeInvalidInput, "„%s“ ist kein Semester. Gib das Ende z. B. als 2026-SS oder 2026-WS an.", *validUntil)
		}
		untilOrd = ord
	}
	if validFrom != nil && validUntil != nil && fromOrd > untilOrd {
		return nil, apierr.Newf(apierr.CodeInvalidInput, "Der Beginn (%s) liegt nach dem Ende (%s). Tausche die beiden Semester.", *validFrom, *validUntil)
	}

	if name == "" {
		if teacher, err := p.dbClient.GetTeacher(ctx, teacherID); err == nil && teacher != nil {
			name = teacher.Fullname
		}
	}
	nonInvigilator := &model.PermanentNonInvigilator{
		TeacherID:  teacherID,
		Name:       name,
		Reason:     reason,
		ValidFrom:  validFrom,
		ValidUntil: validUntil,
	}
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
	curOrd, curOK := semesterOrdinal(p.semester)
	permanentSet := make(map[int]bool, len(permanent))
	for _, n := range permanent {
		if permanentAppliesTo(n, curOrd, curOK) {
			permanentSet[n.TeacherID] = true
		}
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
	// One window per date: the table's key is (semester, teacher, date). The optimizer
	// could handle several, but a second one would fail as a database error.
	windowDates := make(map[string]bool, len(input.TimeWindows))
	for _, w := range input.TimeWindows {
		if w == nil {
			continue
		}
		dateKey := w.Date.In(time.Local).Format("2006-01-02")
		if windowDates[dateKey] {
			return nil, apierr.Newf(apierr.CodeInvalidInput, "Für den %s ist mehr als ein Zeitfenster eingetragen. Je Tag ist nur eines möglich; fasse sie zu einem zusammen.", dayLabelDE(w.Date))
		}
		windowDates[dateKey] = true
		if w.From == nil && w.Until == nil {
			return nil, apierr.Newf(apierr.CodeInvalidInput, "Das Zeitfenster am %s hat weder einen Beginn noch ein Ende. Trage mindestens eines davon ein oder lösche das Zeitfenster.", dayLabelDE(w.Date))
		}
		if w.From != nil && w.Until != nil && !w.Until.After(*w.From) {
			return nil, apierr.Newf(apierr.CodeInvalidInput, "Im Zeitfenster am %s liegt das Ende (%s Uhr) nicht nach dem Beginn (%s Uhr). Korrigiere die Uhrzeiten.", dayLabelDE(w.Date), w.Until.Format("15:04"), w.From.Format("15:04"))
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
