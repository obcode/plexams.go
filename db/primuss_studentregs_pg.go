package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// studentRegFromRow maps a registration onto the model.
//
// Program is the STUDENT's own programme (Primuss column Stg), not the exam's.
// The two live in separate columns because they are separate things -- see the
// table comment and TestStudentRegKeepsBothPrograms. Under MongoDB one was the
// collection name and the other a document field, so nothing forced them
// together; here nothing may merge them either.
func studentRegFromRow(row sqlc.Studentreg) *model.StudentReg {
	return &model.StudentReg{
		Mtknr:         row.Mtknr,
		PrimussAncode: row.PrimussAncode,
		Program:       row.StudentProgram,
		Group:         row.GroupName,
		Name:          row.Name,
		Presence:      row.Presence,
	}
}

func studentRegsFromRows(rows []sqlc.Studentreg) []*model.StudentReg {
	regs := make([]*model.StudentReg, 0, len(rows))
	for _, row := range rows {
		regs = append(regs, studentRegFromRow(row))
	}
	return regs
}

func (db *PG) GetPrimussStudentRegsForProgrammAncode(ctx context.Context, program string, ancode int) ([]*model.StudentReg, error) {
	rows, err := db.q(ctx).ListStudentRegsForAncode(ctx, sqlc.ListStudentRegsForAncodeParams{
		SemesterID:    db.semesterID,
		Program:       program,
		PrimussAncode: ancode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get student regs")
		return nil, err
	}
	return studentRegsFromRows(rows), nil
}

func (db *PG) GetPrimussStudentRegsPerAncode(ctx context.Context, program string) (map[int][]*model.StudentReg, error) {
	rows, err := db.q(ctx).ListStudentRegsForProgram(ctx, sqlc.ListStudentRegsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get student regs")
		return nil, err
	}

	perAncode := make(map[int][]*model.StudentReg)
	for _, row := range rows {
		perAncode[row.PrimussAncode] = append(perAncode[row.PrimussAncode], studentRegFromRow(row))
	}
	return perAncode, nil
}

func (db *PG) GetPrimussStudentRegsPerStudent(ctx context.Context, program string) (map[string][]*model.StudentReg, error) {
	rows, err := db.q(ctx).ListStudentRegsForProgram(ctx, sqlc.ListStudentRegsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get student regs")
		return nil, err
	}

	perStudent := make(map[string][]*model.StudentReg)
	for _, row := range rows {
		perStudent[row.Mtknr] = append(perStudent[row.Mtknr], studentRegFromRow(row))
	}
	return perStudent, nil
}

func (db *PG) StudentRegsForProgram(ctx context.Context, program string) ([]*model.StudentReg, error) {
	rows, err := db.q(ctx).ListStudentRegsForProgram(ctx, sqlc.ListStudentRegsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get student regs")
		return nil, err
	}
	return studentRegsFromRows(rows), nil
}

// ChangeAncodeInStudentRegs renumbers the registrations of an exam.
//
// It still moves the counter first, as before. After ChangeAncode that is a
// no-op (the counter followed the exam by cascade); the registrations, which
// deliberately have no key to primuss_exam, are moved here.
func (db *PG) ChangeAncodeInStudentRegs(ctx context.Context, program string, ancode, newAncode int) ([]*model.StudentReg, error) {
	if err := db.ChangeAncodeInStudentRegsCount(ctx, program, ancode, newAncode); err != nil {
		return nil, err
	}

	err := db.q(ctx).ChangeStudentRegsAncode(ctx, sqlc.ChangeStudentRegsAncodeParams{
		SemesterID:      db.semesterID,
		Program:         program,
		PrimussAncode:   ancode,
		PrimussAncode_2: newAncode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode")
		return nil, err
	}

	return db.GetPrimussStudentRegsForProgrammAncode(ctx, program, newAncode)
}

// DuplicateStudentRegs finds students registered several times for the same exam.
//
// Reported, never enforced: the Primuss source data really does contain such a
// duplicate, so a unique key on (ancode, mtknr) would reject the import instead
// of protecting it. Surfacing it here is what makes the missing constraint
// acceptable.
func (db *PG) DuplicateStudentRegs(ctx context.Context, program string) ([]DuplicateStudentReg, error) {
	rows, err := db.q(ctx).ListDuplicateStudentRegs(ctx, sqlc.ListDuplicateStudentRegsParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get duplicate student regs")
		return nil, err
	}

	duplicates := make([]DuplicateStudentReg, 0, len(rows))
	for _, row := range rows {
		duplicates = append(duplicates, DuplicateStudentReg{
			Program: program,
			Ancode:  row.Ancode,
			Mtknr:   row.Mtknr,
			Count:   row.N,
		})
	}
	return duplicates, nil
}

// RemoveStudentReg deletes one registration and decrements the Primuss counter.
// Both writes run in one transaction, so the counter cannot drift when the second
// one fails -- under MongoDB that needed a replica set and silently degraded to
// two independent writes without one.
func (db *PG) RemoveStudentReg(ctx context.Context, program string, ancode int, mtknr string) (int, error) {
	var deleted int
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		rows, err := db.q(ctx).DeleteOneStudentReg(ctx, sqlc.DeleteOneStudentRegParams{
			SemesterID:    db.semesterID,
			Program:       program,
			PrimussAncode: ancode,
			Mtknr:         mtknr,
		})
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("ancode", ancode).Str("mtknr", mtknr).
				Msg("error while trying to delete")
			return err
		}
		deleted = int(rows)
		if deleted == 0 {
			return nil
		}

		return db.q(ctx).IncPrimussCount(ctx, sqlc.IncPrimussCountParams{
			SemesterID: db.semesterID,
			Program:    program,
			Ancode:     ancode,
			Delta:      -1,
		})
	})
	return deleted, err
}

// AddStudentReg inserts one registration and increments the Primuss counter, in
// one transaction for the same reason as RemoveStudentReg.
//
// The registration carries the student's name but not their programme: the Mongo
// insert wrote AnCode, MTKNR and name and nothing else, so student_program stays
// empty here too.
func (db *PG) AddStudentReg(ctx context.Context, program string, ancode int, mtknr string) error {
	return db.InTransaction(ctx, func(ctx context.Context) error {
		student, err := db.StudentByMtknr(ctx, mtknr)
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("ancode", ancode).Str("mtknr", mtknr).
				Msg("error while trying to get student by mtknr")
			return err
		}

		err = db.q(ctx).InsertStudentReg(ctx, sqlc.InsertStudentRegParams{
			SemesterID:    db.semesterID,
			Program:       program,
			PrimussAncode: ancode,
			Mtknr:         mtknr,
			Name:          student.Name,
		})
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("ancode", ancode).Str("mtknr", mtknr).
				Msg("error while trying to insert")
			return err
		}

		return db.q(ctx).IncPrimussCount(ctx, sqlc.IncPrimussCountParams{
			SemesterID: db.semesterID,
			Program:    program,
			Ancode:     ancode,
			Delta:      1,
		})
	})
}
