package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// assembledExamFormatVersion is the shape of model.AssembledExam this binary
// reads and writes. Same contract as the other jsonb columns: the json tags are
// the GraphQL contract too, so a rename in a .graphqls silently changes the
// storage format. Failing loudly beats an exam whose four nested levels come back
// as zero values -- the generator would run with it.
const assembledExamFormatVersion = 1

// assembledExamFromBlob decodes one cached exam, refusing a blob written by a
// binary with a different idea of the format.
func assembledExamFromBlob(blob []byte, formatVersion int) (*model.AssembledExam, error) {
	if formatVersion != assembledExamFormatVersion {
		return nil, fmt.Errorf("assembled exam was written in format version %d, this binary reads %d",
			formatVersion, assembledExamFormatVersion)
	}

	var exam model.AssembledExam
	if err := json.Unmarshal(blob, &exam); err != nil {
		return nil, err
	}
	return &exam, nil
}

// CacheAssembledExams replaces the cache with a freshly assembled set.
//
// Mongo dropped the collection and re-inserted. Here the rows are upserted and
// the ones no longer assembled deleted, inside one transaction -- so a failing
// write cannot leave the cache empty, and the FK to exam never sees a moment
// with no rows.
func (db *PG) CacheAssembledExams(ctx context.Context, exams []*model.AssembledExam) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if _, err := db.q(ctx).DeleteAssembledExams(ctx, db.semesterID); err != nil {
			return err
		}

		for _, exam := range exams {
			blob, err := json.Marshal(exam)
			if err != nil {
				return err
			}
			if err := db.q(ctx).UpsertAssembledExam(ctx, sqlc.UpsertAssembledExamParams{
				SemesterID:    db.semesterID,
				Ancode:        exam.Ancode,
				Exam:          blob,
				FormatVersion: assembledExamFormatVersion,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot cache assembled exams")
		return err
	}

	log.Debug().Int("count", len(exams)).Msg("successfully inserted assembled exams")
	return nil
}

// CountAssembledExams returns how many assembled exams are currently cached (0
// before the first generation).
func (db *PG) CountAssembledExams(ctx context.Context) (int64, error) {
	n, err := db.q(ctx).CountAssembledExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot count assembled exams")
		return 0, err
	}
	return n, nil
}

// DropAssembledExams removes the cached assembled exams and the state row,
// undoing a generation. Returns how many assembled exams were removed.
func (db *PG) DropAssembledExams(ctx context.Context) (int64, error) {
	var n int64
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		var err error
		n, err = db.q(ctx).DeleteAssembledExams(ctx, db.semesterID)
		if err != nil {
			return err
		}
		return db.q(ctx).DeleteAssembledExamsState(ctx, db.semesterID)
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot drop assembled exams")
		return 0, err
	}
	return n, nil
}

func (db *PG) GetAssembledExams(ctx context.Context) ([]*model.AssembledExam, error) {
	rows, err := db.q(ctx).ListAssembledExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
		return nil, err
	}

	exams := make([]*model.AssembledExam, 0, len(rows))
	for _, row := range rows {
		exam, err := assembledExamFromBlob(row.Exam, row.FormatVersion)
		if err != nil {
			log.Error().Err(err).Msg("cannot decode assembled exams")
			return nil, err
		}
		exams = append(exams, exam)
	}

	return exams, nil
}

func (db *PG) GetAssembledExamsForExamer(ctx context.Context, examerID int) ([]*model.AssembledExam, error) {
	rows, err := db.q(ctx).ListAssembledExamsForExamer(ctx, sqlc.ListAssembledExamsForExamerParams{
		SemesterID: db.semesterID,
		ExamerID:   examerID,
	})
	if err != nil {
		log.Error().Err(err).Int("examer", examerID).Msg("cannot get assembled exams")
		return nil, err
	}

	exams := make([]*model.AssembledExam, 0, len(rows))
	for _, row := range rows {
		exam, err := assembledExamFromBlob(row.Exam, row.FormatVersion)
		if err != nil {
			log.Error().Err(err).Msg("cannot decode assembled exams")
			return nil, err
		}
		exams = append(exams, exam)
	}

	return exams, nil
}

// GetAssembledExam returns one cached exam. Not (nil, nil) when it is missing:
// the Mongo version handed the driver's not-found error straight to the caller,
// and ExamsAt aborts on it.
func (db *PG) GetAssembledExam(ctx context.Context, ancode int) (*model.AssembledExam, error) {
	row, err := db.q(ctx).GetAssembledExam(ctx, sqlc.GetAssembledExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		log.Debug().Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, err
	}
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, err
	}

	exam, err := assembledExamFromBlob(row.Exam, row.FormatVersion)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, err
	}
	return exam, nil
}
