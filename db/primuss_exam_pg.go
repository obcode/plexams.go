package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// primussExamFromRow maps a scanned row onto the model. The `raw` column -- the
// XLSX columns we do not model -- is deliberately not carried into the model: it
// exists so a re-export is lossless, not for the domain to read.
func primussExamFromRow(row sqlc.PrimussExam) *model.PrimussExam {
	return &model.PrimussExam{
		AnCode:     row.Ancode,
		Module:     row.Module,
		MainExamer: row.MainExamer,
		Program:    row.Program,
		ExamType:   row.ExamType,
		Presence:   row.Presence,
	}
}

func primussExamsFromRows(rows []sqlc.PrimussExam) []*model.PrimussExam {
	exams := make([]*model.PrimussExam, 0, len(rows))
	for _, row := range rows {
		exams = append(exams, primussExamFromRow(row))
	}
	return exams
}

// GetPrograms returns the programs that have Primuss exams this semester.
//
// The Mongo version ran a regex over collection names. That is also why
// DropPrimussData had to drop rather than empty them: an empty exams_XY kept the
// program visible. Here "has exams" is the definition.
func (db *PG) GetPrograms(ctx context.Context) ([]string, error) {
	programs, err := db.q(ctx).ListPrimussPrograms(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return nil, err
	}
	return programs, nil
}

// GetPrimussExamsForAncode returns the Primuss exam with this ancode in every
// program that has one. The Mongo version looped over the programs with one
// lookup each and swallowed the misses.
func (db *PG) GetPrimussExamsForAncode(ctx context.Context, ancode int) ([]*model.PrimussExam, error) {
	rows, err := db.q(ctx).ListPrimussExamsForAncode(ctx, sqlc.ListPrimussExamsForAncodeParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot find primuss exams for ancode")
		return nil, err
	}
	return primussExamsFromRows(rows), nil
}

// GetPrimussExam returns one Primuss exam. A missing one is an error, as it was
// under Mongo (mongo.ErrNoDocuments reached the caller): several call sites treat
// the error as "this program does not have that exam".
func (db *PG) GetPrimussExam(ctx context.Context, program string, ancode int) (*model.PrimussExam, error) {
	row, err := db.q(ctx).GetPrimussExam(ctx, sqlc.GetPrimussExamParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     ancode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Not found is a normal, caller-handled outcome (connected exams ask every
		// program), so it stays at debug level to avoid flooding the log.
		log.Debug().Str("program", program).Int("ancode", ancode).Msg("primuss exam not found")
		return nil, err
	}
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot find primuss exam")
		return nil, err
	}

	return primussExamFromRow(row), nil
}

func (db *PG) PrimussExamExists(ctx context.Context, program string, ancode int) (bool, error) {
	exists, err := db.q(ctx).PrimussExamExists(ctx, sqlc.PrimussExamExistsParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).
			Msg("cannot check whether primuss exam exists")
		return false, err
	}
	return exists, nil
}

// ChangeAncode renumbers a Primuss exam within its program.
//
// The counter and both sides of every conflict follow by ON UPDATE CASCADE, so
// this one statement does what used to take three writes plus a $rename over
// field names -- and does it atomically, where the Mongo sequence left the data
// inconsistent in between.
func (db *PG) ChangeAncode(ctx context.Context, program string, ancode, newAncode int) (*model.PrimussExam, error) {
	row, err := db.q(ctx).ChangePrimussExamAncode(ctx, sqlc.ChangePrimussExamAncodeParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     ancode,
		Ancode_2:   newAncode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		log.Debug().Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no exam updated while trying to change ancode")
		// Mongo fell through to a lookup of the new ancode, which is what the
		// caller wants when the rename already happened.
		return db.GetPrimussExam(ctx, program, newAncode)
	}
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode")
		return nil, err
	}

	return primussExamFromRow(row), nil
}

// PrimussExamsForProgram returns all Primuss exams of one program.
func (db *PG) PrimussExamsForProgram(ctx context.Context, program string) ([]*model.PrimussExam, error) {
	rows, err := db.q(ctx).ListPrimussExamsForProgram(ctx, sqlc.ListPrimussExamsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get primuss exams")
		return nil, err
	}
	return primussExamsFromRows(rows), nil
}

// GetPrimussExams returns every program's exams together with their registration
// counts. The count comes from Primuss' own number, not from counting rows -- the
// two drifting apart is a signal the validation report exists to catch.
func (db *PG) GetPrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	programs, err := db.GetPrograms(ctx)
	if err != nil {
		return nil, err
	}

	primussExams := make([]*model.PrimussExamByProgram, 0, len(programs))
	for _, program := range programs {
		exams, err := db.PrimussExamsForProgram(ctx, program)
		if err != nil {
			return nil, err
		}

		examsWithCount := make([]*model.PrimussExamWithCount, 0, len(exams))
		for _, exam := range exams {
			sum, err := db.GetStudentRegsCount(ctx, program, exam.AnCode)
			if err != nil {
				log.Error().Err(err).Str("program", program).Int("ancode", exam.AnCode).
					Msg("cannot get student regs sum")
			}

			examsWithCount = append(examsWithCount, &model.PrimussExamWithCount{
				Ancode:           exam.AnCode,
				Module:           exam.Module,
				MainExamer:       exam.MainExamer,
				Program:          exam.Program,
				ExamType:         exam.ExamType,
				Presence:         exam.Presence,
				StudentRegsCount: sum,
			})
		}

		primussExams = append(primussExams, &model.PrimussExamByProgram{
			Program: program,
			Exams:   examsWithCount,
		})
	}

	return primussExams, nil
}

// DropPrimussData removes all imported Primuss data of this semester and returns
// the programs that had any. The manually maintained ancode overlay
// (exam_primuss_ancode) is NOT touched -- it is hand-entered and would not come
// back with the next import.
//
// primuss_count and primuss_conflict go with the exams by cascade; studentreg has
// no key to them by design and is cleared explicitly.
func (db *PG) DropPrimussData(ctx context.Context) ([]string, error) {
	programs, err := db.GetPrograms(ctx)
	if err != nil {
		return nil, err
	}

	if err := db.q(ctx).DeleteStudentRegsForSemester(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("cannot drop student regs")
		return nil, err
	}
	if err := db.q(ctx).DeletePrimussExamsForSemester(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("cannot drop primuss exams")
		return nil, err
	}

	return programs, nil
}
