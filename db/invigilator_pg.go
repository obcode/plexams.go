package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// invigilationTodosFormatVersion is the shape of model.InvigilationTodos this
// binary reads and writes. jsonb because the todos are derived from everything
// else by PrepareInvigilationTodos -- normalising a cache of data that already
// exists relationally would buy a second source of truth and nothing else.
const invigilationTodosFormatVersion = 1

// CacheInvigilatorTodos stores the computed fair-share summary.
//
// GetInvigilationTodos re-caches on every read, so parallel validation
// subscriptions call this concurrently. Under MongoDB that needed a fixed _id
// and a follow-up DeleteMany to heal the documents an interleaved drop-insert
// left behind; here the primary key is one row per semester, so the upsert is
// the whole story and there is nothing to heal.
func (db *PG) CacheInvigilatorTodos(ctx context.Context, todos *model.InvigilationTodos) error {
	blob, err := json.Marshal(todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot encode invigilator todos")
		return err
	}

	err = db.q(ctx).UpsertInvigilationTodos(ctx, sqlc.UpsertInvigilationTodosParams{
		SemesterID:    db.semesterID,
		Todos:         blob,
		FormatVersion: invigilationTodosFormatVersion,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilator todos")
		return err
	}
	return nil
}

// GetInvigilationTodos returns the cached summary, or (nil, nil) before the
// first generation.
func (db *PG) GetInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	row, err := db.q(ctx).GetInvigilationTodos(ctx, db.semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator todos")
		return nil, err
	}

	if row.FormatVersion != invigilationTodosFormatVersion {
		err := fmt.Errorf("invigilation todos were written in format version %d, this binary reads %d",
			row.FormatVersion, invigilationTodosFormatVersion)
		log.Error().Err(err).Msg("cannot find invigilator todos")
		return nil, err
	}

	var todos model.InvigilationTodos
	if err := json.Unmarshal(row.Todos, &todos); err != nil {
		log.Error().Err(err).Msg("cannot decode invigilator todos")
		return nil, err
	}
	return &todos, nil
}

// invigilatorConstraintsFromRow assembles the model; the excluded dates and time
// windows come from their own tables.
func invigilatorConstraintsFromRow(row sqlc.InvigilatorConstraint) *model.InvigilatorConstraints {
	return &model.InvigilatorConstraints{
		TeacherID:        row.TeacherID,
		IsNotInvigilator: row.IsNotInvigilator,
		ExcludedDates:    make([]time.Time, 0),
		TimeWindows:      make([]*model.InvigilationTimeWindow, 0),
	}
}

// InvigilatorConstraints returns all per-invigilator constraints stored in the
// DB (managed via the GUI), separate from the ZPA-sourced requirements.
//
// Three queries, not three per invigilator -- the same answer the single
// document gave, assembled the other way round.
func (db *PG) InvigilatorConstraints(ctx context.Context) ([]*model.InvigilatorConstraints, error) {
	rows, err := db.q(ctx).ListInvigilatorConstraints(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator constraints")
		return nil, err
	}

	dateRows, err := db.q(ctx).ListInvigilatorExcludedDates(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator constraints")
		return nil, err
	}
	excluded := make(map[int][]time.Time, len(rows))
	for _, row := range dateRows {
		excluded[row.TeacherID] = append(excluded[row.TeacherID], row.ExcludedOn)
	}

	windowRows, err := db.q(ctx).ListInvigilatorTimeWindows(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find invigilator constraints")
		return nil, err
	}
	windows := make(map[int][]*model.InvigilationTimeWindow, len(rows))
	for _, row := range windowRows {
		windows[row.TeacherID] = append(windows[row.TeacherID], &model.InvigilationTimeWindow{
			Date:  row.WindowDate,
			From:  row.AvailableFrom,
			Until: row.AvailableUntil,
		})
	}

	constraints := make([]*model.InvigilatorConstraints, 0, len(rows))
	for _, row := range rows {
		constraint := invigilatorConstraintsFromRow(row)
		if dates, ok := excluded[row.TeacherID]; ok {
			constraint.ExcludedDates = dates
		}
		if tws, ok := windows[row.TeacherID]; ok {
			constraint.TimeWindows = tws
		}
		constraints = append(constraints, constraint)
	}
	return constraints, nil
}

// InvigilatorConstraintsForTeacher returns the constraints of one invigilator,
// or (nil, nil) if none are stored -- including on a read error, which is what
// the Mongo version did (the nolint:nilerr there was deliberate).
func (db *PG) InvigilatorConstraintsForTeacher(ctx context.Context, teacherID int) (*model.InvigilatorConstraints, error) {
	row, err := db.q(ctx).GetInvigilatorConstraint(ctx, sqlc.GetInvigilatorConstraintParams{
		SemesterID: db.semesterID,
		TeacherID:  teacherID,
	})
	if err != nil {
		return nil, nil //nolint:nilerr // no row for the teacher is not an error
	}

	constraint := invigilatorConstraintsFromRow(row)

	dates, err := db.q(ctx).ListInvigilatorExcludedDatesForTeacher(ctx,
		sqlc.ListInvigilatorExcludedDatesForTeacherParams{
			SemesterID: db.semesterID,
			TeacherID:  teacherID,
		})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot get excluded dates")
		return nil, err
	}
	if len(dates) > 0 {
		constraint.ExcludedDates = dates
	}

	windows, err := db.q(ctx).ListInvigilatorTimeWindowsForTeacher(ctx,
		sqlc.ListInvigilatorTimeWindowsForTeacherParams{
			SemesterID: db.semesterID,
			TeacherID:  teacherID,
		})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot get time windows")
		return nil, err
	}
	for _, window := range windows {
		constraint.TimeWindows = append(constraint.TimeWindows, &model.InvigilationTimeWindow{
			Date:  window.WindowDate,
			From:  window.AvailableFrom,
			Until: window.AvailableUntil,
		})
	}

	return constraint, nil
}

// UpsertInvigilatorConstraints creates or replaces the whole constraints record
// of one invigilator (key: teacherID), across its three tables in one
// transaction. Replacing the record replaces its dates and windows, as replacing
// the document did.
func (db *PG) UpsertInvigilatorConstraints(ctx context.Context, constraints *model.InvigilatorConstraints) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).UpsertInvigilatorConstraint(ctx, sqlc.UpsertInvigilatorConstraintParams{
			SemesterID:       db.semesterID,
			TeacherID:        constraints.TeacherID,
			IsNotInvigilator: constraints.IsNotInvigilator,
		}); err != nil {
			return err
		}

		if err := db.q(ctx).DeleteInvigilatorExcludedDates(ctx,
			sqlc.DeleteInvigilatorExcludedDatesParams{
				SemesterID: db.semesterID,
				TeacherID:  constraints.TeacherID,
			}); err != nil {
			return err
		}
		for _, date := range constraints.ExcludedDates {
			if err := db.q(ctx).InsertInvigilatorExcludedDate(ctx,
				sqlc.InsertInvigilatorExcludedDateParams{
					SemesterID: db.semesterID,
					TeacherID:  constraints.TeacherID,
					ExcludedOn: date,
				}); err != nil {
				return err
			}
		}

		if err := db.q(ctx).DeleteInvigilatorTimeWindows(ctx,
			sqlc.DeleteInvigilatorTimeWindowsParams{
				SemesterID: db.semesterID,
				TeacherID:  constraints.TeacherID,
			}); err != nil {
			return err
		}
		for _, window := range constraints.TimeWindows {
			if window == nil {
				continue
			}
			if err := db.q(ctx).InsertInvigilatorTimeWindow(ctx,
				sqlc.InsertInvigilatorTimeWindowParams{
					SemesterID:     db.semesterID,
					TeacherID:      constraints.TeacherID,
					WindowDate:     window.Date,
					AvailableFrom:  window.From,
					AvailableUntil: window.Until,
				}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Int("teacherID", constraints.TeacherID).
			Msg("cannot upsert invigilator constraints")
	}
	return err
}

// DeleteInvigilatorConstraints removes the constraints record of one
// invigilator. Returns false if there was none. The dates and windows cascade.
func (db *PG) DeleteInvigilatorConstraints(ctx context.Context, teacherID int) (bool, error) {
	n, err := db.q(ctx).DeleteInvigilatorConstraint(ctx, sqlc.DeleteInvigilatorConstraintParams{
		SemesterID: db.semesterID,
		TeacherID:  teacherID,
	})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).
			Msg("cannot delete invigilator constraints")
		return false, err
	}
	return n > 0, nil
}
