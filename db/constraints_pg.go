package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// timePointers turns the stored timestamptz[] into the []*time.Time the model
// carries. The pointers are an artefact of gqlgen, not a way of saying "unknown
// day": the array has no NULLs.
func timePointers(times []time.Time) []*time.Time {
	if len(times) == 0 {
		return nil
	}
	out := make([]*time.Time, 0, len(times))
	for i := range times {
		out = append(out, &times[i])
	}
	return out
}

// timeValues is the inverse. A nil element would be a day nobody can name, so it
// is dropped rather than stored as a NULL the array cannot hold.
func timeValues(times []*time.Time) []time.Time {
	out := make([]time.Time, 0, len(times))
	for _, t := range times {
		if t != nil {
			out = append(out, *t)
		}
	}
	return out
}

// constraintsFromRow assembles the model from the exam_constraint row; the
// same-slot list and the room constraints are filled in by the caller, which has
// them from its own query.
func constraintsFromRow(row sqlc.ExamConstraint) *model.Constraints {
	return &model.Constraints{
		Ancode:             row.Ancode,
		NotPlannedByMe:     row.NotPlannedByMe,
		DoNotPublish:       row.DoNotPublish,
		ExcludeDays:        timePointers(row.ExcludeDays),
		PossibleDays:       timePointers(row.PossibleDays),
		FixedDay:           row.FixedDay,
		FixedTime:          row.FixedTime,
		Online:             row.Online,
		Location:           row.Location,
		NotPlannedByMeInFk: row.NotPlannedByMeFk,
	}
}

func roomConstraintsFromRow(row sqlc.ExamRoomConstraint, allowedRooms []string) *model.RoomConstraints {
	if allowedRooms == nil {
		allowedRooms = make([]string, 0)
	}
	return &model.RoomConstraints{
		AllowedRooms:     allowedRooms,
		PlacesWithSocket: row.PlacesWithSocket,
		Lab:              row.Lab,
		Exahm:            row.Exahm,
		Seb:              row.Seb,
		KdpJiraURL:       row.KdpJiraUrl,
		MaxStudents:      row.MaxStudents,
		AdditionalSeats:  row.AdditionalSeats,
		PreExamMinutes:   row.PreExamMinutes,
		PostExamMinutes:  row.PostExamMinutes,
		Comments:         row.Comments,
	}
}

// GetConstraintsForAncode returns one exam's constraints, or (nil, nil) when it
// has none. Never an error for "not found": the Mongo version swallowed every
// error here, and every caller treats nil as "no constraints".
func (db *PG) GetConstraintsForAncode(ctx context.Context, ancode int) (*model.Constraints, error) {
	row, err := db.q(ctx).GetExamConstraint(ctx, sqlc.GetExamConstraintParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		log.Debug().Int("ancode", ancode).Msg("no constraint found")
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get constraint")
		return nil, err
	}

	constraints := constraintsFromRow(row)

	sameSlot, err := db.q(ctx).ListSameSlotForAncode(ctx, sqlc.ListSameSlotForAncodeParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get same slot constraints")
		return nil, err
	}
	if len(sameSlot) > 0 {
		constraints.SameSlot = sameSlot
	}

	roomRow, err := db.q(ctx).GetExamRoomConstraint(ctx, sqlc.GetExamRoomConstraintParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No row, no RoomConstraints -- the pointer stays nil.
	case err != nil:
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get room constraints")
		return nil, err
	default:
		rooms, err := db.q(ctx).ListAllowedRoomsForAncode(ctx, sqlc.ListAllowedRoomsForAncodeParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		})
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get allowed rooms")
			return nil, err
		}
		constraints.RoomConstraints = roomConstraintsFromRow(roomRow, rooms)
	}

	return constraints, nil
}

// GetConstraints returns every exam's constraints, sorted by ancode.
//
// Four queries, not four per exam: the Mongo version decoded one document per
// exam, so this is the same answer assembled the other way round.
func (db *PG) GetConstraints(ctx context.Context) ([]*model.Constraints, error) {
	constraints := make([]*model.Constraints, 0)

	rows, err := db.q(ctx).ListExamConstraints(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
		return constraints, err
	}

	pairs, err := db.q(ctx).ListSameSlotPairs(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get same slot constraints")
		return constraints, err
	}
	sameSlot := make(map[int][]int, len(pairs))
	for _, pair := range pairs {
		sameSlot[pair.Ancode] = append(sameSlot[pair.Ancode], pair.OtherAncode)
		sameSlot[pair.OtherAncode] = append(sameSlot[pair.OtherAncode], pair.Ancode)
	}

	roomRows, err := db.q(ctx).ListExamRoomConstraints(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get room constraints")
		return constraints, err
	}

	allowed, err := db.q(ctx).ListAllowedRooms(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get allowed rooms")
		return constraints, err
	}
	allowedRooms := make(map[int][]string, len(roomRows))
	for _, row := range allowed {
		allowedRooms[row.Ancode] = append(allowedRooms[row.Ancode], row.RoomName)
	}

	roomConstraints := make(map[int]*model.RoomConstraints, len(roomRows))
	for _, row := range roomRows {
		roomConstraints[row.Ancode] = roomConstraintsFromRow(row, allowedRooms[row.Ancode])
	}

	for _, row := range rows {
		constraint := constraintsFromRow(row)
		constraint.SameSlot = sameSlot[row.Ancode]
		constraint.RoomConstraints = roomConstraints[row.Ancode]
		constraints = append(constraints, constraint)
	}

	return constraints, nil
}

// AddConstraints replaces one exam's constraints, across all four tables, in one
// transaction.
//
// It can now fail where MongoDB silently succeeded: the foreign key rejects
// constraints for an exam that does not exist. That is deliberate -- 2026-SS
// carries a constraint for ancode 326, which is in neither zpaexams nor
// non_zpaexams, and the orphan report in plexams/validate_db.go existed to find
// exactly that.
func (db *PG) AddConstraints(ctx context.Context, ancode int, constraints *model.Constraints) (*model.Constraints, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).UpsertExamConstraint(ctx, sqlc.UpsertExamConstraintParams{
			SemesterID:       db.semesterID,
			Ancode:           ancode,
			NotPlannedByMe:   constraints.NotPlannedByMe,
			NotPlannedByMeFk: constraints.NotPlannedByMeInFk,
			DoNotPublish:     constraints.DoNotPublish,
			Online:           constraints.Online,
			Location:         constraints.Location,
			ExcludeDays:      timeValues(constraints.ExcludeDays),
			PossibleDays:     timeValues(constraints.PossibleDays),
			FixedDay:         constraints.FixedDay,
			FixedTime:        constraints.FixedTime,
		}); err != nil {
			return err
		}

		if err := db.q(ctx).DeleteSameSlotForAncode(ctx, sqlc.DeleteSameSlotForAncodeParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		}); err != nil {
			return err
		}
		for _, other := range constraints.SameSlot {
			if other == ancode {
				// A pair with itself is not a constraint; Mongo would have
				// stored it and the union-find would have carried it along.
				continue
			}
			if err := db.q(ctx).InsertSameSlotPair(ctx, sqlc.InsertSameSlotPairParams{
				SemesterID: db.semesterID,
				Column2:    ancode,
				Column3:    other,
			}); err != nil {
				return err
			}
		}

		if constraints.RoomConstraints == nil {
			// The pointer being nil is the constraint being absent, so the row
			// goes -- and the allowed rooms cascade with it.
			return db.q(ctx).DeleteExamRoomConstraint(ctx, sqlc.DeleteExamRoomConstraintParams{
				SemesterID: db.semesterID,
				Ancode:     ancode,
			})
		}

		rc := constraints.RoomConstraints
		if err := db.q(ctx).UpsertExamRoomConstraint(ctx, sqlc.UpsertExamRoomConstraintParams{
			SemesterID:       db.semesterID,
			Ancode:           ancode,
			PlacesWithSocket: rc.PlacesWithSocket,
			Lab:              rc.Lab,
			Exahm:            rc.Exahm,
			Seb:              rc.Seb,
			KdpJiraUrl:       rc.KdpJiraURL,
			MaxStudents:      rc.MaxStudents,
			AdditionalSeats:  rc.AdditionalSeats,
			PreExamMinutes:   rc.PreExamMinutes,
			PostExamMinutes:  rc.PostExamMinutes,
			Comments:         rc.Comments,
		}); err != nil {
			return err
		}

		if err := db.q(ctx).DeleteAllowedRooms(ctx, sqlc.DeleteAllowedRoomsParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		}); err != nil {
			return err
		}
		for _, room := range rc.AllowedRooms {
			if err := db.q(ctx).InsertAllowedRoom(ctx, sqlc.InsertAllowedRoomParams{
				SemesterID: db.semesterID,
				Ancode:     ancode,
				RoomName:   room,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot add constraints")
		return nil, err
	}

	return db.GetConstraintsForAncode(ctx, ancode)
}

// RmConstraints removes an exam's constraints. As in the Mongo version, removing
// what is not there is not an error.
func (db *PG) RmConstraints(ctx context.Context, ancode int) (bool, error) {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteSameSlotForAncode(ctx, sqlc.DeleteSameSlotForAncodeParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		}); err != nil {
			return err
		}
		if err := db.q(ctx).DeleteExamRoomConstraint(ctx, sqlc.DeleteExamRoomConstraintParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		}); err != nil {
			return err
		}
		_, err := db.q(ctx).DeleteExamConstraint(ctx, sqlc.DeleteExamConstraintParams{
			SemesterID: db.semesterID,
			Ancode:     ancode,
		})
		return err
	})
	if err != nil {
		log.Debug().Err(err).Int("ancode", ancode).Msg("cannot delete constraints")
		return false, err
	}
	return true, nil
}

// NotPlannedByMe marks an exam as planned by another faculty, keeping whatever
// else is constrained about it.
func (db *PG) NotPlannedByMe(ctx context.Context, ancode int, inFK *string) (bool, error) {
	return db.setConstraint(ctx, ancode, func(constraints *model.Constraints) {
		constraints.NotPlannedByMe = true
		if inFK != nil && *inFK != "" {
			constraints.NotPlannedByMeInFk = inFK
		}
	})
}

func (db *PG) Online(ctx context.Context, ancode int) (bool, error) {
	return db.setConstraint(ctx, ancode, func(constraints *model.Constraints) {
		constraints.Online = true
	})
}

func (db *PG) Lab(ctx context.Context, ancode int) (bool, error) {
	return db.setRoomConstraint(ctx, ancode, func(rc *model.RoomConstraints) { rc.Lab = true })
}

func (db *PG) Exahm(ctx context.Context, ancode int) (bool, error) {
	return db.setRoomConstraint(ctx, ancode, func(rc *model.RoomConstraints) { rc.Exahm = true })
}

func (db *PG) SafeExamBrowser(ctx context.Context, ancode int) (bool, error) {
	return db.setRoomConstraint(ctx, ancode, func(rc *model.RoomConstraints) { rc.Seb = true })
}

// setConstraint is the read-modify-write the five single-flag setters share. It
// is the shape the Mongo versions had -- read the document, set one field, write
// it back -- kept rather than turned into five one-column UPDATEs, because the
// flag may have to create the constraint that holds it.
func (db *PG) setConstraint(ctx context.Context, ancode int, set func(*model.Constraints)) (bool, error) {
	var ok bool
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		constraints, err := db.GetConstraintsForAncode(ctx, ancode)
		if err != nil {
			return err
		}
		if constraints == nil {
			constraints = &model.Constraints{Ancode: ancode}
		}
		set(constraints)

		_, err = db.AddConstraints(ctx, ancode, constraints)
		if err != nil {
			return err
		}
		ok = true
		return nil
	})
	return ok, err
}

func (db *PG) setRoomConstraint(ctx context.Context, ancode int, set func(*model.RoomConstraints)) (bool, error) {
	return db.setConstraint(ctx, ancode, func(constraints *model.Constraints) {
		if constraints.RoomConstraints == nil {
			constraints.RoomConstraints = &model.RoomConstraints{}
		}
		set(constraints.RoomConstraints)
	})
}
