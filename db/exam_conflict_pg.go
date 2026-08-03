package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// StudentConflictDecisions returns all stored explicit per-student decisions.
//
// The Mongo version had to skip documents with an empty mtknr or decision --
// leftovers of a pair-level rating that no longer exists. Both columns are NOT
// NULL here and decision is checked against ACCEPT/VETO, so there is nothing
// left to filter out.
func (db *PG) StudentConflictDecisions(ctx context.Context) ([]*model.StudentConflictDecision, error) {
	rows, err := db.q(ctx).ListConflictDecisions(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get conflict decisions")
		return nil, err
	}

	out := make([]*model.StudentConflictDecision, 0, len(rows))
	for _, row := range rows {
		out = append(out, &model.StudentConflictDecision{
			Ancode1:  row.Ancode,
			Ancode2:  row.OtherAncode,
			Mtknr:    row.Mtknr,
			Decision: model.ConflictDecision(row.Decision),
		})
	}
	return out, nil
}

// UpsertDecision stores (or replaces) an explicit decision by (ancode1, ancode2,
// mtknr). The ancodes are ordered on the way in, so the same pair given either
// way round is the same decision.
func (db *PG) UpsertDecision(ctx context.Context, ancode1, ancode2 int, mtknr, decision string) error {
	err := db.q(ctx).UpsertConflictDecision(ctx, sqlc.UpsertConflictDecisionParams{
		SemesterID: db.semesterID,
		Column2:    ancode1,
		Column3:    ancode2,
		Mtknr:      mtknr,
		Decision:   decision,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot upsert conflict decision")
		return err
	}
	return nil
}

// DeleteDecision removes an explicit decision by (ancode1, ancode2, mtknr).
func (db *PG) DeleteDecision(ctx context.Context, ancode1, ancode2 int, mtknr string) (bool, error) {
	n, err := db.q(ctx).DeleteConflictDecision(ctx, sqlc.DeleteConflictDecisionParams{
		SemesterID: db.semesterID,
		Column2:    ancode1,
		Column3:    ancode2,
		Mtknr:      mtknr,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot delete conflict decision")
		return false, err
	}
	return n > 0, nil
}

// CanShareSlotPairs returns the exam pairs declared as allowed to share a slot,
// each pair once and with the smaller ancode first.
func (db *PG) CanShareSlotPairs(ctx context.Context) ([][2]int, error) {
	rows, err := db.q(ctx).ListCanShareSlotPairs(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get can-share-slot pairs")
		return nil, err
	}

	out := make([][2]int, 0, len(rows))
	for _, row := range rows {
		out = append(out, [2]int{row.Ancode, row.OtherAncode})
	}
	return out, nil
}

// UpsertCanShareSlot declares that two exams may share a slot.
func (db *PG) UpsertCanShareSlot(ctx context.Context, ancode1, ancode2 int) error {
	err := db.q(ctx).UpsertCanShareSlot(ctx, sqlc.UpsertCanShareSlotParams{
		SemesterID: db.semesterID,
		Column2:    ancode1,
		Column3:    ancode2,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot upsert can-share-slot pair")
		return err
	}
	return nil
}

// DeleteCanShareSlot removes a can-share-slot declaration.
func (db *PG) DeleteCanShareSlot(ctx context.Context, ancode1, ancode2 int) (bool, error) {
	n, err := db.q(ctx).DeleteCanShareSlot(ctx, sqlc.DeleteCanShareSlotParams{
		SemesterID: db.semesterID,
		Column2:    ancode1,
		Column3:    ancode2,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot delete can-share-slot pair")
		return false, err
	}
	return n > 0, nil
}
