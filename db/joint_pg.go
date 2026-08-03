package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// jointExamFromRow maps a joint exam row onto the db-package type. Program comes
// from the row rather than from the collection name it used to be; the live data
// agrees with itself here (Studiengruppe always equalled the collection suffix),
// unlike the registrations, where the two programmes are genuinely different
// things.
func jointExamFromRow(row sqlc.JointExam) *JointExam {
	return &JointExam{
		PrimussAncode:  row.PrimussAncode,
		Module:         row.Module,
		ExamType:       row.ExamType,
		Grading:        row.Grading,
		Duration:       row.DurationMin,
		MainExamer:     row.MainExamer,
		SecondExamer:   row.SecondExamer,
		IsRepeaterExam: row.IsRepeaterExam,
		Program:        row.Program,
		Planer:         row.Planer,
	}
}

// ReplaceJointExamsForProgram replaces a joint program's exams wholesale. They
// are re-importable source data, so a clear-and-refill is the right shape; the
// links that reference them (joint_link) are hand-curated and live in their own
// table without a key to these rows, which is why the refill cannot take them.
func (db *PG) ReplaceJointExamsForProgram(ctx context.Context, program string, exams []*JointExam) error {
	err := db.q(ctx).DeleteJointExamsForProgram(ctx, sqlc.DeleteJointExamsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot drop the joint exams")
		return err
	}

	for _, exam := range exams {
		err := db.q(ctx).InsertJointExam(ctx, sqlc.InsertJointExamParams{
			SemesterID:     db.semesterID,
			Program:        program,
			PrimussAncode:  exam.PrimussAncode,
			Module:         exam.Module,
			ExamType:       exam.ExamType,
			Grading:        exam.Grading,
			DurationMin:    exam.Duration,
			MainExamer:     exam.MainExamer,
			SecondExamer:   exam.SecondExamer,
			IsRepeaterExam: exam.IsRepeaterExam,
			Planer:         exam.Planer,
		})
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("ancode", exam.PrimussAncode).
				Msg("cannot insert joint exam")
			return err
		}
	}
	return nil
}

func (db *PG) JointExamsForProgram(ctx context.Context, program string) ([]*JointExam, error) {
	rows, err := db.q(ctx).ListJointExamsForProgram(ctx, sqlc.ListJointExamsForProgramParams{
		SemesterID: db.semesterID,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get exams for joint program")
		return nil, err
	}

	exams := make([]*JointExam, 0, len(rows))
	for _, row := range rows {
		exams = append(exams, jointExamFromRow(row))
	}
	return exams, nil
}

// JointExam returns one joint exam. A missing one is an error, as under Mongo.
func (db *PG) JointExam(ctx context.Context, program string, ancode int) (*JointExam, error) {
	row, err := db.q(ctx).GetJointExam(ctx, sqlc.GetJointExamParams{
		SemesterID:    db.semesterID,
		Program:       program,
		PrimussAncode: ancode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).
			Msg("cannot get exam for joint program")
		return nil, err
	}
	return jointExamFromRow(row), nil
}

func jointLinkFromRow(row sqlc.JointLink) *JointLink {
	return &JointLink{
		Program:       row.Program,
		PrimussAncode: row.PrimussAncode,
		Kind:          row.Kind,
		Ancode:        row.Ancode,
		Status:        row.Status,
		Source:        row.Source,
		Module:        row.Module,
		MainExamer:    row.MainExamer,
	}
}

func (db *PG) JointLinks(ctx context.Context) ([]*JointLink, error) {
	rows, err := db.q(ctx).ListJointLinks(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get joint links")
		return nil, err
	}

	links := make([]*JointLink, 0, len(rows))
	for _, row := range rows {
		links = append(links, jointLinkFromRow(row))
	}
	return links, nil
}

// JointLink returns one link, or nil when there is none -- "not linked yet" is a
// normal state the callers act on.
func (db *PG) JointLink(ctx context.Context, program string, primussAncode int) (*JointLink, error) {
	row, err := db.q(ctx).GetJointLink(ctx, sqlc.GetJointLinkParams{
		SemesterID:    db.semesterID,
		Program:       program,
		PrimussAncode: primussAncode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("primussAncode", primussAncode).
			Msg("cannot get joint link")
		return nil, err
	}
	return jointLinkFromRow(row), nil
}

// UpsertJointLink stores a link (key: program + primuss ancode).
//
// The schema check ties status to the ancode: a link that says 'linked' must name
// one, and an 'unresolved' one must not. That is the invariant
// ValidateDBReferences used to report after the fact.
func (db *PG) UpsertJointLink(ctx context.Context, link *JointLink) error {
	err := db.q(ctx).UpsertJointLink(ctx, sqlc.UpsertJointLinkParams{
		SemesterID:    db.semesterID,
		Program:       link.Program,
		PrimussAncode: link.PrimussAncode,
		Kind:          link.Kind,
		Ancode:        link.Ancode,
		Status:        link.Status,
		Source:        link.Source,
		Module:        link.Module,
		MainExamer:    link.MainExamer,
	})
	if err != nil {
		log.Error().Err(err).Str("program", link.Program).Int("primussAncode", link.PrimussAncode).
			Msg("cannot upsert joint link")
	}
	return err
}

func (db *PG) DeleteJointLink(ctx context.Context, program string, primussAncode int) error {
	err := db.q(ctx).DeleteJointLink(ctx, sqlc.DeleteJointLinkParams{
		SemesterID:    db.semesterID,
		Program:       program,
		PrimussAncode: primussAncode,
	})
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("primussAncode", primussAncode).
			Msg("cannot delete joint link")
	}
	return err
}
