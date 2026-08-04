package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func studyProgramFromRow(row sqlc.StudyProgram) *model.StudyProgram {
	return &model.StudyProgram{
		Shortname:         row.Shortname,
		Name:              row.Name,
		Degree:            row.Degree,
		ZpaCode:           row.ZpaCode,
		Category:          row.Category,
		Active:            row.Active,
		Retired:           row.Retired,
		ExternalExamsBase: row.ExternalExamsBase,
		JointFaculty:      row.JointFaculty,
	}
}

func studyProgramsFromRows(rows []sqlc.StudyProgram) []*model.StudyProgram {
	programs := make([]*model.StudyProgram, 0, len(rows))
	for _, row := range rows {
		programs = append(programs, studyProgramFromRow(row))
	}
	return programs
}

// StudyPrograms returns all study programs (Studiengänge). This list is global
// and carries over between semesters.
func (db *PG) StudyPrograms(ctx context.Context) ([]*model.StudyProgram, error) {
	rows, err := db.q(ctx).ListStudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot find study programs")
		return nil, err
	}

	return studyProgramsFromRows(rows), nil
}

// StudyProgram returns one study program by its shortname, or nil when none.
func (db *PG) StudyProgram(ctx context.Context, shortname string) (*model.StudyProgram, error) {
	row, err := db.q(ctx).GetStudyProgram(ctx, shortname)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("shortname", shortname).Msg("cannot get study program")
		return nil, err
	}

	return studyProgramFromRow(row), nil
}

// UpsertStudyProgram creates or replaces one study program (key: shortname).
func (db *PG) UpsertStudyProgram(ctx context.Context, program *model.StudyProgram) error {
	err := db.q(ctx).UpsertStudyProgram(ctx, sqlc.UpsertStudyProgramParams{
		Shortname:         program.Shortname,
		Name:              program.Name,
		Degree:            program.Degree,
		ZpaCode:           program.ZpaCode,
		Category:          program.Category,
		Active:            program.Active,
		Retired:           program.Retired,
		ExternalExamsBase: program.ExternalExamsBase,
		JointFaculty:      program.JointFaculty,
	})
	if err != nil {
		log.Error().Err(err).Str("shortname", program.Shortname).Msg("cannot upsert study program")
		return err
	}
	return nil
}

// DeleteStudyProgram removes one study program. Returns false if there was none.
//
// New behaviour against Mongo, and intentionally so: exams, Primuss exams,
// registrations and counts reference study_program(shortname), so a program that
// is still in use cannot be deleted out from under them. Mongo let the delete
// succeed and left the references dangling.
func (db *PG) DeleteStudyProgram(ctx context.Context, shortname string) (bool, error) {
	rows, err := db.q(ctx).DeleteStudyProgram(ctx, shortname)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrForeignKeyViolation {
			return false, fmt.Errorf(
				"study program %s is still referenced by exams or Primuss data and cannot be deleted",
				shortname)
		}
		log.Error().Err(err).Str("shortname", shortname).Msg("cannot delete study program")
		return false, err
	}
	return rows > 0, nil
}
