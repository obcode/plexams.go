package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// AddAncode records a manually added Primuss ancode mapping for a ZPA exam.
//
// The Mongo ReplaceOne filtered on (ancode, program), so setting a different
// Primuss ancode for the same program replaced the mapping instead of adding a
// second one; a unique index enforced it. Here that is an insert followed by a
// delete of the other added mappings for that program. In that order on purpose:
// a crash in between leaves a duplicate, which the next call cleans up, rather
// than no mapping at all.
//
// New against Mongo: exam_primuss_ancode references the exam, so a mapping cannot
// be recorded for an ancode that has no exam. Verified against three semesters of
// live data -- every stored mapping points at an existing ZPA exam.
func (db *PG) AddAncode(ctx context.Context, zpaAncode int, program string, primussAncode int) error {
	err := db.q(ctx).AddPrimussAncode(ctx, sqlc.AddPrimussAncodeParams{
		SemesterID:    db.semesterID,
		Ancode:        zpaAncode,
		Program:       program,
		PrimussAncode: primussAncode,
	})
	if err != nil {
		log.Error().Err(err).Int("zpaAncode", zpaAncode).Str("program", program).
			Int("primussAncode", primussAncode).Msg("cannot add primuss ancode for zpa ancode")
		return err
	}

	err = db.q(ctx).DeleteOtherAddedPrimussAncodes(ctx, sqlc.DeleteOtherAddedPrimussAncodesParams{
		SemesterID:    db.semesterID,
		Ancode:        zpaAncode,
		Program:       program,
		PrimussAncode: primussAncode,
	})
	if err != nil {
		log.Error().Err(err).Int("zpaAncode", zpaAncode).Str("program", program).
			Msg("cannot remove the superseded primuss ancode mapping")
		return err
	}

	return nil
}

// RemoveAddedAncode removes a manually added Primuss ancode mapping (program) of a
// ZPA exam. Returns false when there was none. Mappings that come from ZPA itself
// carry source 'zpa' and are not affected.
func (db *PG) RemoveAddedAncode(ctx context.Context, zpaAncode int, program string) (bool, error) {
	rows, err := db.q(ctx).RemoveAddedPrimussAncode(ctx, sqlc.RemoveAddedPrimussAncodeParams{
		SemesterID: db.semesterID,
		Ancode:     zpaAncode,
		Program:    program,
	})
	if err != nil {
		log.Error().Err(err).Int("zpaAncode", zpaAncode).Str("program", program).
			Msg("cannot remove added primuss ancode")
		return false, err
	}
	return rows > 0, nil
}

func (db *PG) GetAddedAncodes(ctx context.Context) (map[int][]model.ZPAPrimussAncodes, error) {
	rows, err := db.q(ctx).ListAddedPrimussAncodes(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get added ancodes")
		return nil, err
	}

	added := make(map[int][]model.ZPAPrimussAncodes)
	for _, row := range rows {
		added[row.Ancode] = append(added[row.Ancode], model.ZPAPrimussAncodes{
			Program: row.Program,
			Ancode:  row.PrimussAncode,
		})
	}
	return added, nil
}

func (db *PG) GetAddedAncodesForAncode(ctx context.Context, ancode int) ([]model.ZPAPrimussAncodes, error) {
	rows, err := db.q(ctx).ListAddedPrimussAncodesForAncode(ctx,
		sqlc.ListAddedPrimussAncodesForAncodeParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get added ancodes")
		return nil, err
	}

	added := make([]model.ZPAPrimussAncodes, 0, len(rows))
	for _, row := range rows {
		added = append(added, model.ZPAPrimussAncodes{
			Program: row.Program,
			Ancode:  row.PrimussAncode,
		})
	}
	return added, nil
}
