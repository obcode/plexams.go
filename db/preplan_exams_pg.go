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

// preplanConstraintsFormatVersion is the shape of the model.Constraints stored
// in preplan_exam.constraints. Same contract as the other jsonb columns: the
// json tags are also the GraphQL contract, so a rename in a .graphqls changes
// the storage format without touching anything that looks like storage.
//
// The embedded Constraints is jsonb rather than a second copy of the four
// exam_constraint tables: it is an optional sub-document read and written with
// its row, and nothing joins it.
const preplanConstraintsFormatVersion = 1

func preplanExamFromRow(row sqlc.PreplanExam) (*model.PreplanExam, error) {
	exam := &model.PreplanExam{
		ID:               row.ID,
		ExamKind:         row.ExamKind,
		ExamerID:         row.ExamerID,
		ExamerName:       row.ExamerName,
		Module:           row.Module,
		Programs:         row.Programs,
		ExpectedStudents: row.ExpectedStudents,
		Duration:         row.DurationMin,
		PlannedStarttime: row.PlannedStarttime,
		IsFixed:          row.IsFixed,
		Ancode:           row.Ancode,
		Notes:            row.Notes,
	}
	if exam.Programs == nil {
		exam.Programs = make([]string, 0)
	}

	if len(row.Constraints) > 0 {
		if row.FormatVersion != preplanConstraintsFormatVersion {
			return nil, fmt.Errorf("pre-exam %d constraints were written in format version %d, this binary reads %d",
				row.ID, row.FormatVersion, preplanConstraintsFormatVersion)
		}
		var constraints model.Constraints
		if err := json.Unmarshal(row.Constraints, &constraints); err != nil {
			return nil, err
		}
		exam.Constraints = &constraints
	}

	return exam, nil
}

// preplanPairs reads both pair relations at once and returns them per pre-exam
// id, in both directions -- the model carries a list on each side and plexams
// keeps them symmetric.
func (db *PG) preplanPairs(ctx context.Context) (notSameSlot, canShareSlot map[int][]int, err error) {
	notSame, err := db.q(ctx).ListPreplanNotSameSlot(ctx, db.semesterID)
	if err != nil {
		return nil, nil, err
	}
	canShare, err := db.q(ctx).ListPreplanCanShareSlot(ctx, db.semesterID)
	if err != nil {
		return nil, nil, err
	}

	notSameSlot = make(map[int][]int, len(notSame))
	for _, pair := range notSame {
		notSameSlot[pair.ID] = append(notSameSlot[pair.ID], pair.OtherID)
		notSameSlot[pair.OtherID] = append(notSameSlot[pair.OtherID], pair.ID)
	}
	canShareSlot = make(map[int][]int, len(canShare))
	for _, pair := range canShare {
		canShareSlot[pair.ID] = append(canShareSlot[pair.ID], pair.OtherID)
		canShareSlot[pair.OtherID] = append(canShareSlot[pair.OtherID], pair.ID)
	}
	return notSameSlot, canShareSlot, nil
}

// PreplanExams returns all SEB/EXaHM pre-planning pseudo-exams of this semester,
// sorted by id.
func (db *PG) PreplanExams(ctx context.Context) ([]*model.PreplanExam, error) {
	rows, err := db.q(ctx).ListPreplanExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find pre-exams")
		return nil, err
	}

	notSameSlot, canShareSlot, err := db.preplanPairs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot find pre-exams")
		return nil, err
	}

	exams := make([]*model.PreplanExam, 0, len(rows))
	for _, row := range rows {
		exam, err := preplanExamFromRow(row)
		if err != nil {
			log.Error().Err(err).Msg("cannot decode pre-exams")
			return nil, err
		}
		exam.NotSameSlot = notSameSlot[row.ID]
		exam.CanShareSlot = canShareSlot[row.ID]
		exams = append(exams, exam)
	}
	return exams, nil
}

// PreplanExam returns one pre-exam by id, or (nil, nil) when there is none.
func (db *PG) PreplanExam(ctx context.Context, id int) (*model.PreplanExam, error) {
	row, err := db.q(ctx).GetPreplanExam(ctx, sqlc.GetPreplanExamParams{
		SemesterID: db.semesterID,
		ID:         id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot get pre-exam")
		return nil, err
	}

	exam, err := preplanExamFromRow(row)
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot get pre-exam")
		return nil, err
	}

	notSameSlot, canShareSlot, err := db.preplanPairs(ctx)
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot get pre-exam")
		return nil, err
	}
	exam.NotSameSlot = notSameSlot[id]
	exam.CanShareSlot = canShareSlot[id]

	return exam, nil
}

// InsertPreplanExam assigns the next id and inserts the pre-exam.
//
// Reading max(id)+1 and inserting happen in one transaction, so two creations at
// once cannot pick the same id -- under MongoDB they could, and the loser
// silently became a second pre-exam with a duplicate id that the pair tables
// could not tell apart.
func (db *PG) InsertPreplanExam(ctx context.Context, preplanExam *model.PreplanExam) (*model.PreplanExam, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		id, err := db.q(ctx).NextPreplanExamID(ctx, db.semesterID)
		if err != nil {
			log.Error().Err(err).Msg("cannot determine next pre-exam id")
			return err
		}
		preplanExam.ID = id
		return db.writePreplanExam(ctx, preplanExam)
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot insert pre-exam")
		return nil, err
	}
	return preplanExam, nil
}

// ReplacePreplanExam replaces the pre-exam with the same id. Returns false if
// there was none -- it does not create one, which is what separates it from
// UpsertPreplanExam.
func (db *PG) ReplacePreplanExam(ctx context.Context, preplanExam *model.PreplanExam) (bool, error) {
	var existed bool
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		var err error
		existed, err = db.q(ctx).PreplanExamExists(ctx, sqlc.PreplanExamExistsParams{
			SemesterID: db.semesterID,
			ID:         preplanExam.ID,
		})
		if err != nil || !existed {
			return err
		}
		return db.writePreplanExam(ctx, preplanExam)
	})
	if err != nil {
		log.Error().Err(err).Int("id", preplanExam.ID).Msg("cannot replace pre-exam")
		return false, err
	}
	return existed, nil
}

// UpsertPreplanExam inserts or replaces a pre-exam keeping its explicit id
// (unlike InsertPreplanExam, which assigns a fresh one). Used by the CSV import
// so that the id references in notSameSlot/canShareSlot stay valid across a
// re-import.
func (db *PG) UpsertPreplanExam(ctx context.Context, preplanExam *model.PreplanExam) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		return db.writePreplanExam(ctx, preplanExam)
	})
	if err != nil {
		log.Error().Err(err).Int("id", preplanExam.ID).Msg("cannot upsert pre-exam")
	}
	return err
}

// writePreplanExam writes the row and this pre-exam's side of both pair
// relations. Must run inside a transaction.
//
// Replacing a pre-exam replaces its whole side of each relation, which is what
// replacing the Mongo document did. A pair whose other half is not (yet) stored
// is rejected by the foreign key -- so the CSV import has to write the pre-exams
// before their pairs, and does.
func (db *PG) writePreplanExam(ctx context.Context, exam *model.PreplanExam) error {
	var constraints []byte
	if exam.Constraints != nil {
		blob, err := json.Marshal(exam.Constraints)
		if err != nil {
			return err
		}
		constraints = blob
	}

	programs := exam.Programs
	if programs == nil {
		programs = make([]string, 0)
	}

	if err := db.q(ctx).UpsertPreplanExam(ctx, sqlc.UpsertPreplanExamParams{
		SemesterID:       db.semesterID,
		ID:               exam.ID,
		ExamKind:         exam.ExamKind,
		ExamerID:         exam.ExamerID,
		ExamerName:       exam.ExamerName,
		Module:           exam.Module,
		Programs:         programs,
		ExpectedStudents: exam.ExpectedStudents,
		DurationMin:      exam.Duration,
		PlannedStarttime: exam.PlannedStarttime,
		IsFixed:          exam.IsFixed,
		Ancode:           exam.Ancode,
		Notes:            exam.Notes,
		Constraints:      constraints,
		FormatVersion:    preplanConstraintsFormatVersion,
	}); err != nil {
		return err
	}

	if err := db.q(ctx).DeletePreplanNotSameSlotFor(ctx, sqlc.DeletePreplanNotSameSlotForParams{
		SemesterID: db.semesterID,
		ID:         exam.ID,
	}); err != nil {
		return err
	}
	for _, other := range exam.NotSameSlot {
		if other == exam.ID {
			continue
		}
		if err := db.q(ctx).InsertPreplanNotSameSlot(ctx, sqlc.InsertPreplanNotSameSlotParams{
			SemesterID: db.semesterID,
			Column2:    exam.ID,
			Column3:    other,
		}); err != nil {
			return err
		}
	}

	if err := db.q(ctx).DeletePreplanCanShareSlotFor(ctx, sqlc.DeletePreplanCanShareSlotForParams{
		SemesterID: db.semesterID,
		ID:         exam.ID,
	}); err != nil {
		return err
	}
	for _, other := range exam.CanShareSlot {
		if other == exam.ID {
			continue
		}
		if err := db.q(ctx).InsertPreplanCanShareSlot(ctx, sqlc.InsertPreplanCanShareSlotParams{
			SemesterID: db.semesterID,
			Column2:    exam.ID,
			Column3:    other,
		}); err != nil {
			return err
		}
	}

	return nil
}

// DeletePreplanExam removes one pre-exam. Returns false if there was none. Its
// pairs go with it -- both pair tables cascade.
func (db *PG) DeletePreplanExam(ctx context.Context, id int) (bool, error) {
	n, err := db.q(ctx).DeletePreplanExam(ctx, sqlc.DeletePreplanExamParams{
		SemesterID: db.semesterID,
		ID:         id,
	})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot delete pre-exam")
		return false, err
	}
	return n > 0, nil
}
