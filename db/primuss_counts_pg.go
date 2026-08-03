package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// GetStudentRegsCount returns Primuss' own recorded number of registrations for an
// exam, or 0 when Primuss has no counter for it.
func (db *PG) GetStudentRegsCount(ctx context.Context, program string, ancode int) (int, error) {
	total, err := db.q(ctx).GetPrimussCount(ctx, sqlc.GetPrimussCountParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     ancode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("error finding count")
		return -1, err
	}
	return total, nil
}

// ChangeAncodeInStudentRegsCount renumbers the counter of an exam.
//
// After ChangeAncode this is already done -- the counter follows the exam by
// ON UPDATE CASCADE -- and the statement then matches nothing. It stays because
// ChangeAncodeInStudentRegs calls it directly and must keep working on its own.
func (db *PG) ChangeAncodeInStudentRegsCount(ctx context.Context, program string, ancode, newAncode int) error {
	err := db.q(ctx).ChangePrimussCountAncode(ctx, sqlc.ChangePrimussCountAncodeParams{
		SemesterID: db.semesterID,
		Program:    program,
		Ancode:     ancode,
		Ancode_2:   newAncode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode in count")
		return err
	}
	return nil
}

// StudentRegsCountMismatches reports the exams whose stored registrations
// disagree with Primuss' recorded count.
//
// Only exams that have registrations are considered, exactly as before: a counter
// with no registrations at all is not reported. Recorded is NoCountDocument when
// Primuss delivered no counter for an exam that does have registrations.
func (db *PG) StudentRegsCountMismatches(ctx context.Context, program string) ([]StudentRegsCountMismatch, error) {
	rows, err := db.q(ctx).ListStudentRegsCountMismatches(ctx,
		sqlc.ListStudentRegsCountMismatchesParams{
			SemesterID: db.semesterID,
			Program:    program,
		})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get student regs count mismatches")
		return nil, err
	}

	mismatches := make([]StudentRegsCountMismatch, 0, len(rows))
	for _, row := range rows {
		recorded := NoCountDocument
		if row.Total != nil {
			recorded = *row.Total
		}
		mismatches = append(mismatches, StudentRegsCountMismatch{
			Program:  program,
			Ancode:   row.Ancode,
			Stored:   row.Stored,
			Recorded: recorded,
		})
	}
	return mismatches, nil
}
