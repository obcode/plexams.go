package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// AddExternalExam stores an exam owned by another faculty or a joint program.
//
// It writes the same table as the ZPA exams, with source 'external'. That is
// what makes a plan entry or a constraint able to reference "an exam" at all:
// under MongoDB `non_zpaexams` was a second collection, and validate_db.go had to
// maintain a knownAncodes set by hand to say whether an ancode was real.
func (db *PG) AddExternalExam(ctx context.Context, exam *model.ZPAExam) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		zpaID := exam.ZpaID
		groups := exam.Groups
		if groups == nil {
			groups = make([]string, 0)
		}

		if err := db.q(ctx).InsertExternalExam(ctx, sqlc.InsertExternalExamParams{
			SemesterID:     db.semesterID,
			Ancode:         exam.AnCode,
			ZpaID:          &zpaID,
			Module:         exam.Module,
			MainExamer:     exam.MainExamer,
			MainExamerID:   exam.MainExamerID,
			ExamType:       exam.ExamType,
			ExamTypeFull:   exam.ExamTypeFull,
			ZpaDate:        exam.Date,
			ZpaStarttime:   exam.Starttime,
			DurationMin:    exam.Duration,
			IsRepeaterExam: exam.IsRepeaterExam,
			Groups:         groups,
			Faculty:        exam.Faculty,
		}); err != nil {
			return err
		}

		for _, primuss := range exam.PrimussAncodes {
			if err := db.q(ctx).InsertExternalPrimussAncode(ctx, sqlc.InsertExternalPrimussAncodeParams{
				SemesterID:    db.semesterID,
				Ancode:        exam.AnCode,
				Program:       primuss.Program,
				PrimussAncode: primuss.Ancode,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot add external exam")
	}
	return err
}

// DeleteExternalExam removes an externally owned exam by its ancode.
//
// This is a deliberate act, so the cascade is correct here: the exam's plan
// entry, rooms and constraints go with it. That is exactly the cascade the ZPA
// import must not trigger, which is why the import marks instead of deleting.
func (db *PG) DeleteExternalExam(ctx context.Context, ancode int) error {
	err := db.q(ctx).DeleteExternalExam(ctx, sqlc.DeleteExternalExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot delete non zpa exam")
	}
	return err
}

// RemovePlanEntry removes the plan entry (if any) of an ancode.
func (db *PG) RemovePlanEntry(ctx context.Context, ancode int) error {
	err := db.q(ctx).DeletePlanEntry(ctx, sqlc.DeletePlanEntryParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot remove plan entry")
	}
	return err
}

// SetExternalExamFaculty sets the faculty (Prüfungsplanung) on an external exam.
func (db *PG) SetExternalExamFaculty(ctx context.Context, ancode int, faculty string) error {
	err := db.q(ctx).SetExternalExamFaculty(ctx, sqlc.SetExternalExamFacultyParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
		Faculty:    faculty,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot set external exam faculty")
	}
	return err
}

// ExternalExam returns one externally owned exam. Missing is an error, not
// (nil, nil) -- the Mongo version handed the driver's not-found error through,
// and plexams/joint.go reads it as "not added yet".
func (db *PG) ExternalExam(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	row, err := db.q(ctx).GetExternalExam(ctx, sqlc.GetExternalExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exam")
		return nil, err
	}

	semester, err := db.semesterLabel(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot resolve the semester label")
		return nil, err
	}

	exam := zpaExamFromRow(row, semester)

	links, err := db.q(ctx).ListPrimussAncodesForExternalExam(ctx, sqlc.ListPrimussAncodesForExternalExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get non zpa exam")
		return nil, err
	}
	for _, link := range links {
		exam.PrimussAncodes = append(exam.PrimussAncodes, model.ZPAPrimussAncodes{
			Program: link.Program,
			Ancode:  link.PrimussAncode,
		})
	}

	return exam, nil
}

func (db *PG) ExternalExams(ctx context.Context) ([]*model.ZPAExam, error) {
	rows, err := db.q(ctx).ListExternalExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exams")
		return nil, err
	}

	semester, err := db.semesterLabel(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot resolve the semester label")
		return nil, err
	}

	links, err := db.q(ctx).ListPrimussAncodesForExternalExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get non zpa exams")
		return nil, err
	}
	perAncode := make(map[int][]model.ZPAPrimussAncodes, len(rows))
	for _, link := range links {
		perAncode[link.Ancode] = append(perAncode[link.Ancode], model.ZPAPrimussAncodes{
			Program: link.Program,
			Ancode:  link.PrimussAncode,
		})
	}

	exams := make([]*model.ZPAExam, 0, len(rows))
	for _, row := range rows {
		exam := zpaExamFromRow(row, semester)
		if primuss, ok := perAncode[row.Ancode]; ok {
			exam.PrimussAncodes = primuss
		}
		exams = append(exams, exam)
	}
	return exams, nil
}
