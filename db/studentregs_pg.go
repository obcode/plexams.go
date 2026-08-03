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

// studentPreparedFormatVersion is the shape of model.Student this binary reads
// and writes into student_prepared. Bump it when a field is renamed or changes
// meaning; adding one is handled by encoding/json.
const studentPreparedFormatVersion = 1

// regWithErrorFormatVersion does the same for the ZPA upload rejects.
const regWithErrorFormatVersion = 1

func studentFromJSON(blob []byte, formatVersion int) (*model.Student, error) {
	if formatVersion != studentPreparedFormatVersion {
		return nil, fmt.Errorf("prepared student was written in format version %d, this binary reads %d",
			formatVersion, studentPreparedFormatVersion)
	}
	var student model.Student
	if err := json.Unmarshal(blob, &student); err != nil {
		return nil, err
	}
	return &student, nil
}

// CountStudentRegsPlanned returns how many planned student registrations are
// currently prepared (0 before the first preparation).
func (db *PG) CountStudentRegsPlanned(ctx context.Context) (int64, error) {
	n, err := db.q(ctx).CountStudentsPrepared(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot count prepared student regs")
		return 0, err
	}
	return n, nil
}

func (db *PG) StudentRegsPerStudentPlanned(ctx context.Context) ([]*model.Student, error) {
	rows, err := db.q(ctx).ListStudentsPrepared(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get prepared student regs")
		return nil, err
	}

	students := make([]*model.Student, 0, len(rows))
	for _, row := range rows {
		student, err := studentFromJSON(row.Student, row.FormatVersion)
		if err != nil {
			log.Error().Err(err).Msg("cannot decode prepared student")
			return nil, err
		}
		students = append(students, student)
	}
	return students, nil
}

// StudentByMtknr returns one prepared student. A missing one is an error, as it
// was under Mongo -- AddStudentReg reads the name from here and must not invent
// a registration for a student nobody has prepared.
func (db *PG) StudentByMtknr(ctx context.Context, mtknr string) (*model.Student, error) {
	row, err := db.q(ctx).GetStudentPrepared(ctx, sqlc.GetStudentPreparedParams{
		SemesterID: db.semesterID,
		Mtknr:      mtknr,
	})
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).Msg("cannot find student by mtknr")
		return nil, err
	}
	return studentFromJSON(row.Student, row.FormatVersion)
}

// StudentsByName finds prepared students whose name matches the regex.
//
// The Mongo version ran the regex over every studentregs_<program> collection,
// collected the Matrikelnummern into a set and then did one lookup per student.
// This is the same thing as two queries. The regex is POSIX here rather than
// MongoDB's flavour, which for name searches is the same language.
func (db *PG) StudentsByName(ctx context.Context, regex string) ([]*model.Student, error) {
	mtknrs, err := db.q(ctx).ListStudentMtknrsByName(ctx, sqlc.ListStudentMtknrsByNameParams{
		SemesterID: db.semesterID,
		Name:       regex,
	})
	if err != nil {
		log.Error().Err(err).Str("regex", regex).Msg("error while searching for students")
		return nil, err
	}
	if len(mtknrs) == 0 {
		return make([]*model.Student, 0), nil
	}

	rows, err := db.q(ctx).ListStudentsPreparedByMtknr(ctx, sqlc.ListStudentsPreparedByMtknrParams{
		SemesterID: db.semesterID,
		Mtknrs:     mtknrs,
	})
	if err != nil {
		log.Error().Err(err).Msg("error while trying to get students")
		return nil, err
	}

	// A student who matched but has not been prepared is skipped, exactly as the
	// Mongo loop did when its per-student lookup failed.
	students := make([]*model.Student, 0, len(rows))
	for _, row := range rows {
		student, err := studentFromJSON(row.Student, row.FormatVersion)
		if err != nil {
			log.Error().Err(err).Msg("cannot decode prepared student")
			continue
		}
		students = append(students, student)
	}
	return students, nil
}

// SaveStudentRegs replaces the prepared per-student view.
//
// Like its Mongo predecessor this is a clear-and-refill and is only atomic when
// the caller wraps it in a transaction; the state row is what records whether the
// cache is trustworthy.
func (db *PG) SaveStudentRegs(ctx context.Context, studentRegs []interface{}) error {
	if err := db.q(ctx).DeleteStudentsPrepared(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("error while trying to drop the prepared students")
		return err
	}

	for _, raw := range studentRegs {
		student, ok := raw.(*model.Student)
		if !ok {
			return fmt.Errorf("cannot save a %T as a prepared student", raw)
		}
		blob, err := json.Marshal(student)
		if err != nil {
			log.Error().Err(err).Str("mtknr", student.Mtknr).Msg("cannot encode prepared student")
			return err
		}
		err = db.q(ctx).InsertStudentPrepared(ctx, sqlc.InsertStudentPreparedParams{
			SemesterID:    db.semesterID,
			Mtknr:         student.Mtknr,
			Student:       blob,
			FormatVersion: studentPreparedFormatVersion,
		})
		if err != nil {
			log.Error().Err(err).Str("mtknr", student.Mtknr).Msg("error while trying to insert")
			return err
		}
	}
	return nil
}

// SetStudentRegsDirty records that the prepared registrations went stale
// (dirty=true) or were just rebuilt (dirty=false), with the operation that did it.
func (db *PG) SetStudentRegsDirty(ctx context.Context, dirty bool, reason string, t time.Time) error {
	params := sqlc.SetStudentRegsStateParams{
		SemesterID: db.semesterID,
		Dirty:      dirty,
		ChangedAt:  &t,
	}
	if reason != "" {
		params.Reason = &reason
	}

	if err := db.q(ctx).SetStudentRegsState(ctx, params); err != nil {
		log.Error().Err(err).Bool("dirty", dirty).Msg("cannot set student-regs state")
		return err
	}
	return nil
}

// GetStudentRegsState returns the student-regs state; a missing row means nothing
// has been generated yet and is reported as not dirty.
func (db *PG) GetStudentRegsState(ctx context.Context) (*model.StudentRegsState, error) {
	row, err := db.q(ctx).GetStudentRegsState(ctx, db.semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &model.StudentRegsState{Dirty: false}, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get student-regs state")
		return nil, err
	}

	return &model.StudentRegsState{
		Dirty:     row.Dirty,
		Reason:    row.Reason,
		ChangedAt: row.ChangedAt,
	}, nil
}

// SetRegsWithErrors replaces the recorded ZPA upload rejects.
func (db *PG) SetRegsWithErrors(ctx context.Context, regsWithErrors []*model.RegWithError) error {
	if err := db.q(ctx).DeleteStudentRegUploadErrors(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("cannot drop the student reg upload errors")
		return err
	}

	for _, regWithError := range regsWithErrors {
		registration, err := json.Marshal(regWithError.Registration)
		if err != nil {
			return err
		}
		failure, err := json.Marshal(regWithError.Error)
		if err != nil {
			return err
		}
		err = db.q(ctx).InsertStudentRegUploadError(ctx, sqlc.InsertStudentRegUploadErrorParams{
			SemesterID:    db.semesterID,
			Registration:  registration,
			Error:         failure,
			FormatVersion: regWithErrorFormatVersion,
		})
		if err != nil {
			log.Error().Err(err).Msg("cannot insert student reg upload error")
			return err
		}
	}
	return nil
}

func (db *PG) GetRegsWithErrors(ctx context.Context) ([]*model.RegWithError, error) {
	rows, err := db.q(ctx).ListStudentRegUploadErrors(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get the student reg upload errors")
		return nil, err
	}

	regsWithErrors := make([]*model.RegWithError, 0, len(rows))
	for _, row := range rows {
		if row.FormatVersion != regWithErrorFormatVersion {
			return nil, fmt.Errorf("upload error was written in format version %d, this binary reads %d",
				row.FormatVersion, regWithErrorFormatVersion)
		}
		var regWithError model.RegWithError
		if err := json.Unmarshal(row.Registration, &regWithError.Registration); err != nil {
			log.Error().Err(err).Msg("cannot decode the rejected registration")
			return nil, err
		}
		if err := json.Unmarshal(row.Error, &regWithError.Error); err != nil {
			log.Error().Err(err).Msg("cannot decode the rejection")
			return nil, err
		}
		regsWithErrors = append(regsWithErrors, &regWithError)
	}
	return regsWithErrors, nil
}

// NtasWithRegs returns the prepared students that have an NTA, sorted by the
// NTA's name -- the mailing list for the NTA notifications.
//
// Reads the prepared per-student view, as the Mongo version did: the NTA there
// is the one resolved for this semester, together with the student's actual
// registrations.
func (db *PG) NtasWithRegs(ctx context.Context) ([]*model.Student, error) {
	rows, err := db.q(ctx).ListStudentsWithNta(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas with regs")
		return nil, err
	}

	students := make([]*model.Student, 0, len(rows))
	for _, row := range rows {
		student, err := studentFromJSON(row.Student, row.FormatVersion)
		if err != nil {
			log.Error().Err(err).Msg("cannot decode ntas with regs")
			return nil, err
		}
		students = append(students, student)
	}
	return students, nil
}
