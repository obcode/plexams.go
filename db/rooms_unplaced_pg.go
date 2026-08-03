package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// ReplaceUnplacedExams rewrites the unplaced students of the last room
// generation, in one transaction -- the Mongo version dropped the collection
// first, so a failing insert left it empty.
func (db *PG) ReplaceUnplacedExams(ctx context.Context, unplaced []*model.UnplacedExam) error {
	return db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteUnplacedExams(ctx, db.semesterID); err != nil {
			log.Error().Err(err).Msg("cannot drop unplaced exams")
			return err
		}

		for _, u := range unplaced {
			mtknrs := u.Mtknrs
			if mtknrs == nil {
				mtknrs = make([]string, 0)
			}
			if err := db.q(ctx).UpsertUnplacedExam(ctx, sqlc.UpsertUnplacedExamParams{
				SemesterID: db.semesterID,
				Ancode:     u.Ancode,
				Mtknrs:     mtknrs,
				NtaMtknr:   u.NtaMtknr,
			}); err != nil {
				log.Error().Err(err).Int("ancode", u.Ancode).Msg("cannot insert unplaced exams")
				return err
			}
		}
		return nil
	})
}

// UnplacedExams returns the students that could not be assigned a real room in
// their slot during the last room generation.
func (db *PG) UnplacedExams(ctx context.Context) ([]*model.UnplacedExam, error) {
	rows, err := db.q(ctx).ListUnplacedExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find unplaced exams")
		return nil, err
	}

	unplaced := make([]*model.UnplacedExam, 0, len(rows))
	for _, row := range rows {
		mtknrs := row.Mtknrs
		if mtknrs == nil {
			mtknrs = make([]string, 0)
		}
		unplaced = append(unplaced, &model.UnplacedExam{
			Starttime: row.Starttime,
			Ancode:    row.Ancode,
			Mtknrs:    mtknrs,
			NtaMtknr:  row.NtaMtknr,
		})
	}
	return unplaced, nil
}

// ResetUnplacedExams clears the unplaced students.
func (db *PG) ResetUnplacedExams(ctx context.Context) error {
	if err := db.q(ctx).DeleteUnplacedExams(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("cannot drop unplaced exams")
		return err
	}
	return nil
}
