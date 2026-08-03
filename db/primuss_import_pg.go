package db

import (
	"context"
	"encoding/json"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// rawJSON encodes the unmodelled XLSX columns. An empty map is stored as {}, not
// NULL, so the column is always readable.
func rawJSON(raw map[string]any) ([]byte, error) {
	if raw == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(raw)
}

// ReplacePrimussExams writes the Prüfungskatalog of one program.
//
// The program is the one from the FILE NAME, not the `Stg` column of the rows:
// the file is what defines the per-program namespace, and with the degree
// suffixes a Prüfungskatalog-DC-B file carries `Stg = DC` for every row. The
// column goes into raw, where it stays visible without being mistaken for the key.
func (db *PG) ReplacePrimussExams(ctx context.Context, program string, rows []PrimussExamRow) (int, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeletePrimussExamsOfProgram(ctx, sqlc.DeletePrimussExamsOfProgramParams{
			SemesterID: db.semesterID,
			Program:    program,
		}); err != nil {
			return err
		}
		for _, row := range rows {
			raw, err := rawJSON(row.Raw)
			if err != nil {
				return err
			}
			if err := db.q(ctx).InsertPrimussExam(ctx, sqlc.InsertPrimussExamParams{
				SemesterID: db.semesterID,
				Program:    program,
				Ancode:     row.Ancode,
				Module:     row.Module,
				MainExamer: row.MainExamer,
				ExamType:   row.ExamType,
				Presence:   row.Presence,
				Raw:        raw,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot replace primuss exams")
		return 0, err
	}
	return len(rows), nil
}

// ReplacePrimussStudentRegs writes the Prüfungsanmeldungen of one program.
//
// There is deliberately no unique key on (ancode, mtknr): the Primuss source
// data contains real double registrations, and a unique index would reject the
// import instead of protecting anything. They are reported by
// DuplicateStudentRegs, as before.
func (db *PG) ReplacePrimussStudentRegs(ctx context.Context, program string, rows []PrimussStudentRegRow) (int, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteStudentRegsOfProgram(ctx, sqlc.DeleteStudentRegsOfProgramParams{
			SemesterID: db.semesterID,
			Program:    program,
		}); err != nil {
			return err
		}
		for _, row := range rows {
			raw, err := rawJSON(row.Raw)
			if err != nil {
				return err
			}
			if err := db.q(ctx).ImportStudentReg(ctx, sqlc.ImportStudentRegParams{
				SemesterID:     db.semesterID,
				Program:        program,
				StudentProgram: row.StudentProgram,
				PrimussAncode:  row.PrimussAncode,
				Mtknr:          row.Mtknr,
				GroupName:      row.Group,
				Name:           row.Name,
				Presence:       row.Presence,
				Aaspf:          row.Aaspf,
				Raw:            raw,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot replace student regs")
		return 0, err
	}
	return len(rows), nil
}

// ReplacePrimussCounts writes the Prüfungsplanung of one program. The
// per-study-group breakdown stays in raw: it is a wide row of one column per
// group, read only as a whole.
func (db *PG) ReplacePrimussCounts(ctx context.Context, program string, rows []PrimussCountRow) (int, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeletePrimussCountsOfProgram(ctx, sqlc.DeletePrimussCountsOfProgramParams{
			SemesterID: db.semesterID,
			Program:    program,
		}); err != nil {
			return err
		}
		for _, row := range rows {
			raw, err := rawJSON(row.Raw)
			if err != nil {
				return err
			}
			if err := db.q(ctx).InsertPrimussCount(ctx, sqlc.InsertPrimussCountParams{
				SemesterID: db.semesterID,
				Program:    program,
				Ancode:     row.Ancode,
				Total:      row.Total,
				Raw:        raw,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot replace primuss counts")
		return 0, err
	}
	return len(rows), nil
}

// ReplacePrimussConflicts writes the conflicts of one program, already pivoted
// from the wide XLSX into one row per pair.
//
// This is where problem 3 of the plan is finally paid off: the counterpart
// ancodes are values, not field names, so the hand-written decoder and the
// $rename over field names are both gone -- and both foreign keys make a
// conflict that names an exam outside the catalogue impossible rather than
// invisible.
func (db *PG) ReplacePrimussConflicts(ctx context.Context, program string, rows []PrimussConflictRow) (int, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeletePrimussConflictsOfProgram(ctx, sqlc.DeletePrimussConflictsOfProgramParams{
			SemesterID: db.semesterID,
			Program:    program,
		}); err != nil {
			return err
		}
		for _, row := range rows {
			if err := db.q(ctx).InsertPrimussConflict(ctx, sqlc.InsertPrimussConflictParams{
				SemesterID:  db.semesterID,
				Program:     program,
				Ancode:      row.Ancode,
				OtherAncode: row.OtherAncode,
				NumStudents: row.NumStudents,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot replace primuss conflicts")
		return 0, err
	}
	return len(rows), nil
}

// PrimussAncodesOfProgram returns the ancodes the catalogue of a program knows,
// so the importer can report a conflict naming an exam that is not in it.
func (db *PG) PrimussAncodesOfProgram(ctx context.Context, program string) ([]int, error) {
	ancodes, err := db.q(ctx).ListPrimussAncodesOfProgram(ctx, sqlc.ListPrimussAncodesOfProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot list primuss ancodes")
		return nil, err
	}
	return ancodes, nil
}

// PrimussStudentRegRows reads back the stored registrations of a program, for
// the change detection on re-import.
func (db *PG) PrimussStudentRegRows(ctx context.Context, program string) ([]PrimussStudentRegRow, error) {
	dbRows, err := db.q(ctx).ListStudentRegRowsOfProgram(ctx, sqlc.ListStudentRegRowsOfProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get student regs")
		return nil, err
	}

	rows := make([]PrimussStudentRegRow, 0, len(dbRows))
	for _, dbRow := range dbRows {
		row := PrimussStudentRegRow{
			Mtknr:          dbRow.Mtknr,
			PrimussAncode:  dbRow.PrimussAncode,
			StudentProgram: dbRow.StudentProgram,
			Group:          dbRow.GroupName,
			Name:           dbRow.Name,
			Presence:       dbRow.Presence,
			Aaspf:          dbRow.Aaspf,
		}
		if len(dbRow.Raw) > 0 {
			if err := json.Unmarshal(dbRow.Raw, &row.Raw); err != nil {
				log.Error().Err(err).Str("program", program).Msg("cannot decode student reg raw columns")
				return nil, err
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
