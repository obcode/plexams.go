package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
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

// PermanentNonInvigilators returns the teachers who never do invigilation duty
// again (global, carries over between semesters).
func (p *Plexams) PermanentNonInvigilators(ctx context.Context) ([]*model.PermanentNonInvigilator, error) {
	return p.dbClient.PermanentNonInvigilators(ctx)
}

// SetPermanentNonInvigilator adds or updates a permanent (cross-semester)
// non-invigilator.
func (p *Plexams) SetPermanentNonInvigilator(ctx context.Context, teacherID int, reason string) (*model.PermanentNonInvigilator, error) {
	nonInvigilator := &model.PermanentNonInvigilator{TeacherID: teacherID, Reason: reason}
	if err := p.dbClient.UpsertPermanentNonInvigilator(ctx, nonInvigilator); err != nil {
		return nil, err
	}
	return nonInvigilator, nil
}

// RemovePermanentNonInvigilator removes a permanent non-invigilator.
func (p *Plexams) RemovePermanentNonInvigilator(ctx context.Context, teacherID int) (bool, error) {
	return p.dbClient.DeletePermanentNonInvigilator(ctx, teacherID)
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
	return constraints, nil
}

// DeleteInvigilatorConstraints removes the constraints record of one invigilator.
func (p *Plexams) DeleteInvigilatorConstraints(ctx context.Context, teacherID int) (bool, error) {
	return p.dbClient.DeleteInvigilatorConstraints(ctx, teacherID)
}

// MigrateInvigilatorConstraintsFromConfig is a one-time migration that copies the
// invigilatorConstraints from the semester config (viper) into the DB collection.
// Returns the number of records written.
func (p *Plexams) MigrateInvigilatorConstraintsFromConfig(ctx context.Context) (int, error) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	raw := viper.GetStringMap("invigilatorConstraints")
	count := 0
	for key := range raw {
		var teacherID int
		if _, err := fmt.Sscanf(key, "%d", &teacherID); err != nil {
			log.Warn().Str("key", key).Msg("invigilatorConstraints: cannot parse teacher id, skipped")
			continue
		}

		base := fmt.Sprintf("invigilatorConstraints.%s", key)
		isNot := viper.GetBool(base + ".isNotInvigilator")

		excludedDates := make([]time.Time, 0)
		for _, day := range viper.GetStringSlice(base + ".excludedDates") {
			t, err := time.ParseInLocation("02.01.06", day, loc)
			if err != nil {
				log.Error().Err(err).Str("day", day).Int("teacherID", teacherID).Msg("migrate: cannot parse excluded date")
				continue
			}
			excludedDates = append(excludedDates, t)
		}

		var timeWindows []*model.InvigilationTimeWindow
		if twCfg := viper.Get(base + ".timeWindows"); twCfg != nil {
			tw, err := timeWindowsFromConfig(twCfg)
			if err != nil {
				return count, fmt.Errorf("migrate invigilatorConstraints for %d: %w", teacherID, err)
			}
			timeWindows = tw
		}

		constraints := &model.InvigilatorConstraints{
			TeacherID:        teacherID,
			IsNotInvigilator: isNot,
			ExcludedDates:    excludedDates,
			TimeWindows:      timeWindows,
		}
		if err := p.dbClient.UpsertInvigilatorConstraints(ctx, constraints); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
