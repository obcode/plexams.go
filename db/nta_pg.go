package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// ntaFromRow maps a scanned row onto the GraphQL model. The column names differ
// from the model field names in two places: from/until are reserved-ish words in
// SQL and became valid_from/valid_until.
func ntaFromRow(row sqlc.NTARow) *model.NTA {
	return &model.NTA{
		Name:                 row.Name,
		Email:                row.Email,
		Mtknr:                row.Mtknr,
		Compensation:         row.Compensation,
		DeltaDurationPercent: row.DeltaDurationPercent,
		NeedsRoomAlone:       row.NeedsRoomAlone,
		NeedsHardware:        row.NeedsHardware,
		Program:              row.Program,
		From:                 row.ValidFrom,
		Until:                row.ValidUntil,
		LastSemester:         row.LastSemester,
		Deactivated:          row.Deactivated,
	}
}

func ntasFromRows(rows []sqlc.NTARow) []*model.NTA {
	ntas := make([]*model.NTA, 0, len(rows))
	for _, row := range rows {
		ntas = append(ntas, ntaFromRow(row))
	}
	return ntas
}

func (db *PG) AddNta(ctx context.Context, nta *model.NTA) (*model.NTA, error) {
	row, err := db.q(ctx).InsertNta(ctx, sqlc.InsertNtaParams{
		Mtknr:                nta.Mtknr,
		Name:                 nta.Name,
		Email:                nta.Email,
		Compensation:         nta.Compensation,
		DeltaDurationPercent: nta.DeltaDurationPercent,
		NeedsRoomAlone:       nta.NeedsRoomAlone,
		NeedsHardware:        nta.NeedsHardware,
		Program:              nta.Program,
		ValidFrom:            nta.From,
		ValidUntil:           nta.Until,
		LastSemester:         nta.LastSemester,
		Deactivated:          nta.Deactivated,
	})
	if err != nil {
		log.Error().Err(err).Interface("nta", nta).Msg("cannot insert nta into DB")
		return nil, err
	}

	return ntaFromRow(row), nil
}

// ReplaceNta replaces the NTA identified by its mtknr. It does not upsert: if no
// row matches, nothing is changed and (nil, nil) is returned.
func (db *PG) ReplaceNta(ctx context.Context, nta *model.NTA) (*model.NTA, error) {
	row, err := db.q(ctx).ReplaceNta(ctx, sqlc.ReplaceNtaParams{
		Mtknr:                nta.Mtknr,
		Name:                 nta.Name,
		Email:                nta.Email,
		Compensation:         nta.Compensation,
		DeltaDurationPercent: nta.DeltaDurationPercent,
		NeedsRoomAlone:       nta.NeedsRoomAlone,
		NeedsHardware:        nta.NeedsHardware,
		Program:              nta.Program,
		ValidFrom:            nta.From,
		ValidUntil:           nta.Until,
		LastSemester:         nta.LastSemester,
		Deactivated:          nta.Deactivated,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Interface("nta", nta).Msg("cannot replace nta in DB")
		return nil, err
	}

	return ntaFromRow(row), nil
}

// SetNtaDeactivated sets the deactivated flag of the NTA identified by mtknr.
// Returns nil if no NTA with that mtknr exists.
func (db *PG) SetNtaDeactivated(ctx context.Context, mtknr string, deactivated bool) (*model.NTA, error) {
	row, err := db.q(ctx).SetNtaDeactivated(ctx, sqlc.SetNtaDeactivatedParams{
		Mtknr:       mtknr,
		Deactivated: deactivated,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).Msg("cannot set nta deactivated flag")
		return nil, err
	}

	return ntaFromRow(row), nil
}

func (db *PG) Nta(ctx context.Context, mtknr string) (*model.NTA, error) {
	row, err := db.q(ctx).GetNta(ctx, mtknr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).Msg("error while finding nta")
		return nil, err
	}

	return ntaFromRow(row), nil
}

func (db *PG) Ntas(ctx context.Context) ([]*model.NTA, error) {
	rows, err := db.q(ctx).ListNtas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return nil, err
	}

	return ntasFromRows(rows), nil
}

// SetSemesterOnNTAs stamps the current semester onto every NTA that appears in
// this semester's registrations, so a compensation that has not been used for a
// while can be spotted.
//
// The label written is the LOGICAL semester ("2026 SS"), not the workspace id
// ("2026-WS") -- all 63 stored values have that shape. PG carries only the
// workspace id, so the label is resolved through the registry rather than by
// adding a second process-global field.
//
// The Mongo version issued one FindOneAndUpdate per student; this is one
// statement, and `returning` still names the students who have no NTA row, which
// is what the per-student log message was for.
func (db *PG) SetSemesterOnNTAs(ctx context.Context, studentRegs []interface{}) error {
	mtknrs := make([]string, 0, len(studentRegs))
	for _, regRaw := range studentRegs {
		reg := regRaw.(*model.Student)
		if reg.Nta == nil {
			continue
		}
		mtknrs = append(mtknrs, reg.Mtknr)
	}
	if len(mtknrs) == 0 {
		return nil
	}

	label, err := db.q(ctx).GetSemesterLabel(ctx, db.semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Mongo wrote db.semester unconditionally; here a workspace missing from
		// the registry has to be loud rather than silently stamping nothing.
		return fmt.Errorf("cannot set semester on ntas: %s is not in the semester registry", db.semesterID)
	}
	if err != nil {
		log.Error().Err(err).Str("semesterID", db.semesterID).Msg("cannot resolve the semester label")
		return err
	}

	updated, err := db.q(ctx).SetLastSemesterOnNtas(ctx, sqlc.SetLastSemesterOnNtasParams{
		LastSemester: &label,
		Mtknrs:       mtknrs,
	})
	if err != nil {
		log.Error().Err(err).Str("semester", label).Msg("error when setting semester on ntas")
		return err
	}

	found := make(map[string]bool, len(updated))
	for _, mtknr := range updated {
		found[mtknr] = true
	}
	for _, mtknr := range mtknrs {
		if !found[mtknr] {
			log.Error().Str("mtknr", mtknr).Msg("nta with mtknr not found")
		}
	}
	log.Debug().Str("last semester", label).Int("ntas", len(updated)).
		Msg("last semester set on ntas")

	return nil
}
