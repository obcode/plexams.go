package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func permanentNonInvigilatorFromRow(row sqlc.PermanentNonInvigilator) *model.PermanentNonInvigilator {
	return &model.PermanentNonInvigilator{
		TeacherID:  row.TeacherID,
		Name:       row.Name,
		Reason:     row.Reason,
		ValidFrom:  row.ValidFrom,
		ValidUntil: row.ValidUntil,
	}
}

// PermanentNonInvigilators returns the teachers who never do invigilation duty
// again. This list is global and carries over between semesters.
func (db *PG) PermanentNonInvigilators(ctx context.Context) ([]*model.PermanentNonInvigilator, error) {
	rows, err := db.q(ctx).ListPermanentNonInvigilators(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot find permanent non-invigilators")
		return nil, err
	}

	nonInvigilators := make([]*model.PermanentNonInvigilator, 0, len(rows))
	for _, row := range rows {
		nonInvigilators = append(nonInvigilators, permanentNonInvigilatorFromRow(row))
	}
	return nonInvigilators, nil
}

// UpsertPermanentNonInvigilator creates or replaces one permanent non-invigilator
// (key: teacherID).
func (db *PG) UpsertPermanentNonInvigilator(ctx context.Context, nonInvigilator *model.PermanentNonInvigilator) error {
	err := db.q(ctx).UpsertPermanentNonInvigilator(ctx, sqlc.UpsertPermanentNonInvigilatorParams{
		TeacherID:  nonInvigilator.TeacherID,
		Name:       nonInvigilator.Name,
		Reason:     nonInvigilator.Reason,
		ValidFrom:  nonInvigilator.ValidFrom,
		ValidUntil: nonInvigilator.ValidUntil,
	})
	if err != nil {
		log.Error().Err(err).Int("teacherID", nonInvigilator.TeacherID).
			Msg("cannot upsert permanent non-invigilator")
		return err
	}
	return nil
}

// DeletePermanentNonInvigilator removes one permanent non-invigilator. Returns
// false if there was none.
func (db *PG) DeletePermanentNonInvigilator(ctx context.Context, teacherID int) (bool, error) {
	rows, err := db.q(ctx).DeletePermanentNonInvigilator(ctx, teacherID)
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot delete permanent non-invigilator")
		return false, err
	}
	return rows > 0, nil
}
