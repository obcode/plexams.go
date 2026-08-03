package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

// reserveRoom is the pseudo-room name callers pass for a duty that is not tied
// to one room. It never reaches the database: such a duty is a NULL room_name
// plus is_reserve.
const reserveRoom = "reserve"

func invigilationFromRow(row sqlc.Invigilation) *model.Invigilation {
	starttime := row.Starttime
	invigilation := &model.Invigilation{
		Starttime:          &starttime,
		RoomName:           row.RoomName,
		Duration:           row.DurationMin,
		InvigilatorID:      row.InvigilatorID,
		IsReserve:          row.IsReserve,
		IsSelfInvigilation: row.IsSelfInvigilation,
		PrePlanned:         row.PrePlanned,
	}
	// Slot is derived on read, never stored -- the same decoration the Mongo
	// layer applied in decorateInvigilation.
	invigilation.Slot = &model.Slot{Starttime: starttime}
	return invigilation
}

func invigilationsFromRows(rows []sqlc.Invigilation) []*model.Invigilation {
	invigilations := make([]*model.Invigilation, 0, len(rows))
	for _, row := range rows {
		invigilations = append(invigilations, invigilationFromRow(row))
	}
	return invigilations
}

// GetInvigilatorAt returns the invigilator of a room (or the reserve) at a start
// time, or (nil, nil) when nobody is assigned. More than one is an error, as
// before -- it means the generation produced something contradictory.
func (db *PG) GetInvigilatorAt(ctx context.Context, roomname string, starttime time.Time) (*model.Teacher, error) {
	invigilations, err := db.GetInvigilationsAt(ctx, roomname, starttime)
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Time("starttime", starttime).
			Msg("cannot get invigilations")
		return nil, err
	}

	if len(invigilations) > 1 {
		log.Error().Str("room", roomname).Time("starttime", starttime).
			Interface("invigilations", invigilations).
			Msg("found more than one invigilation")
		return nil, fmt.Errorf("found more than one invigilation")
	}
	if len(invigilations) == 0 {
		return nil, nil
	}

	return db.GetTeacher(ctx, invigilations[0].InvigilatorID)
}

// GetInvigilationsAt returns the invigilations of a room (or the reserve) at a
// start time, self and generated together -- one query where the Mongo version
// concatenated two collections.
func (db *PG) GetInvigilationsAt(ctx context.Context, roomname string, starttime time.Time) ([]*model.Invigilation, error) {
	if roomname == reserveRoom {
		rows, err := db.q(ctx).ListReserveInvigilationsAt(ctx, sqlc.ListReserveInvigilationsAtParams{
			SemesterID: db.semesterID,
			Starttime:  starttime,
		})
		if err != nil {
			log.Error().Err(err).Time("starttime", starttime).
				Msg("cannot get reserve invigilations")
			return nil, err
		}
		return invigilationsFromRows(rows), nil
	}

	rows, err := db.q(ctx).ListInvigilationsInRoomAt(ctx, sqlc.ListInvigilationsInRoomAtParams{
		SemesterID: db.semesterID,
		Starttime:  starttime,
		RoomName:   &roomname,
	})
	if err != nil {
		log.Error().Err(err).Str("room", roomname).Time("starttime", starttime).
			Msg("cannot get invigilations")
		return nil, err
	}
	return invigilationsFromRows(rows), nil
}

func (db *PG) InvigilationsForInvigilator(ctx context.Context, invigilatorID int) ([]*model.Invigilation, error) {
	rows, err := db.q(ctx).ListInvigilationsForInvigilator(ctx, sqlc.ListInvigilationsForInvigilatorParams{
		SemesterID:    db.semesterID,
		InvigilatorID: invigilatorID,
	})
	if err != nil {
		log.Error().Err(err).Int("invigilator-id", invigilatorID).
			Msg("cannot get invigilations for invigilator")
		return nil, err
	}
	return invigilationsFromRows(rows), nil
}

func (db *PG) GetAllInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	rows, err := db.q(ctx).ListInvigilations(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilations")
		return nil, err
	}
	return invigilationsFromRows(rows), nil
}

func (db *PG) GetSelfInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	return db.invigilationsBySelf(ctx, true)
}

func (db *PG) GetOtherInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	return db.invigilationsBySelf(ctx, false)
}

func (db *PG) invigilationsBySelf(ctx context.Context, self bool) ([]*model.Invigilation, error) {
	rows, err := db.q(ctx).ListInvigilationsBySelf(ctx, sqlc.ListInvigilationsBySelfParams{
		SemesterID:         db.semesterID,
		IsSelfInvigilation: self,
	})
	if err != nil {
		log.Error().Err(err).Bool("self", self).Msg("cannot get invigilations")
		return nil, err
	}
	return invigilationsFromRows(rows), nil
}

// AddInvigilationAt assigns an invigilator to a room (or the reserve) at a start
// time, replacing whoever was there -- the Mongo version was an upserting
// ReplaceOne on that filter, so a second call moves the duty instead of adding
// one. Delete plus insert in one transaction is that replace; there is no unique
// key to hang an upsert on, because generated invigilations legitimately repeat
// per room and slot across the semester.
//
// The duration is the slot's time block, not the credited 60 minutes a reserve
// gets in PrepareInvigilationTodos.
func (db *PG) AddInvigilationAt(ctx context.Context, room string, starttime time.Time, invigilatorID int) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		var (
			roomname  *string
			duration  int
			isReserve bool
		)
		if room == reserveRoom {
			duration = db.getMaxDurationAt(ctx, starttime)
			isReserve = true
		} else {
			duration = db.getMaxDurationForRoomAt(ctx, room, starttime)
			roomname = &room
		}

		if err := db.q(ctx).DeleteInvigilationInRoomAt(ctx, sqlc.DeleteInvigilationInRoomAtParams{
			SemesterID: db.semesterID,
			Starttime:  starttime,
			RoomName:   roomname,
			IsReserve:  isReserve,
		}); err != nil {
			return err
		}

		return db.q(ctx).InsertGeneratedInvigilation(ctx, sqlc.InsertGeneratedInvigilationParams{
			SemesterID:    db.semesterID,
			InvigilatorID: invigilatorID,
			Starttime:     starttime,
			RoomName:      roomname,
			DurationMin:   duration,
			IsReserve:     isReserve,
		})
	})
	if err != nil {
		log.Error().Err(err).Str("room", room).Time("starttime", starttime).
			Int("invigilator-id", invigilatorID).Msg("cannot add  invigilation")
	}
	return err
}

// SetInvigilationPrePlannedAt sets the prePlanned flag on the generated
// invigilation for a room (roomName != nil) or the reserve (roomName == nil) at
// a start time. Finding nothing is an error, as before.
func (db *PG) SetInvigilationPrePlannedAt(ctx context.Context, starttime time.Time, roomName *string, prePlanned bool) error {
	n, err := db.q(ctx).SetInvigilationPrePlannedAt(ctx, sqlc.SetInvigilationPrePlannedAtParams{
		SemesterID: db.semesterID,
		Starttime:  starttime,
		RoomName:   roomName,
		IsReserve:  roomName == nil,
		PrePlanned: prePlanned,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).
			Msg("cannot set prePlanned on invigilation")
		return err
	}
	if n == 0 {
		return fmt.Errorf("no invigilation found to mark as pre-planned at %s",
			starttime.Format("02.01. 15:04"))
	}
	return nil
}

// getMaxDurationForRoomAt is the longest planned use of one room at a start time.
func (db *PG) getMaxDurationForRoomAt(ctx context.Context, roomname string, starttime time.Time) int {
	maxDuration := 0

	examsInSlot, _ := db.ExamsAt(ctx, starttime)
	for _, exam := range examsInSlot {
		for _, room := range exam.PlannedRooms {
			if roomname == room.RoomName && maxDuration < room.Duration {
				maxDuration = room.Duration
			}
		}
	}

	return maxDuration
}

// getMaxDurationAt returns the longest invigilation (room duration) across all
// rooms at the start time, used as the time block for a reserve invigilation.
func (db *PG) getMaxDurationAt(ctx context.Context, starttime time.Time) int {
	maxDuration := 0

	examsInSlot, _ := db.ExamsAt(ctx, starttime)
	for _, exam := range examsInSlot {
		for _, room := range exam.PlannedRooms {
			if maxDuration < room.Duration {
				maxDuration = room.Duration
			}
		}
	}

	return maxDuration
}

func prePlannedInvigilationFromRow(row sqlc.PrePlannedInvigilation) *model.PrePlannedInvigilation {
	starttime := row.Starttime
	return &model.PrePlannedInvigilation{
		Starttime:     &starttime,
		InvigilatorID: row.InvigilatorID,
		RoomName:      row.RoomName,
		IsReserve:     row.IsReserve,
	}
}

func (db *PG) PrePlannedInvigilations(ctx context.Context) ([]*model.PrePlannedInvigilation, error) {
	rows, err := db.q(ctx).ListPrePlannedInvigilations(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre planned invigilations")
		return nil, err
	}

	invigilations := make([]*model.PrePlannedInvigilation, 0, len(rows))
	for _, row := range rows {
		invigilations = append(invigilations, prePlannedInvigilationFromRow(row))
	}
	return invigilations, nil
}

func (db *PG) PrePlannedInvigilationsForInvigilator(ctx context.Context, invigilatorID int) ([]*model.PrePlannedInvigilation, error) {
	rows, err := db.q(ctx).ListPrePlannedInvigilationsForInvigilator(ctx,
		sqlc.ListPrePlannedInvigilationsForInvigilatorParams{
			SemesterID:    db.semesterID,
			InvigilatorID: invigilatorID,
		})
	if err != nil {
		log.Error().Err(err).Int("invigilator-id", invigilatorID).
			Msg("cannot get pre planned invigilations")
		return nil, err
	}

	invigilations := make([]*model.PrePlannedInvigilation, 0, len(rows))
	for _, row := range rows {
		invigilations = append(invigilations, prePlannedInvigilationFromRow(row))
	}
	return invigilations, nil
}

// AddPrePlannedInvigilation pins an invigilator to a room (or the reserve) at a
// start time. Only one invigilator per room and slot: assigning a second person
// replaces the first, which is what the delete-then-insert did.
func (db *PG) AddPrePlannedInvigilation(ctx context.Context, prePlannedInvigilation *model.PrePlannedInvigilation) (bool, error) {
	// The absolute start time is the source of truth; the caller sets it.
	if prePlannedInvigilation.Starttime == nil {
		return false, fmt.Errorf("pre-planned invigilation has no start time")
	}

	err := db.q(ctx).UpsertPrePlannedInvigilation(ctx, sqlc.UpsertPrePlannedInvigilationParams{
		SemesterID:    db.semesterID,
		InvigilatorID: prePlannedInvigilation.InvigilatorID,
		Starttime:     *prePlannedInvigilation.Starttime,
		RoomName:      prePlannedInvigilation.RoomName,
		IsReserve:     prePlannedInvigilation.IsReserve,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", *prePlannedInvigilation.Starttime).
			Int("invigilator-id", prePlannedInvigilation.InvigilatorID).
			Msg("cannot insert pre planned invigilation")
		return false, err
	}
	return true, nil
}

// ResetGeneratedInvigilations clears the generated invigilations. The
// pre-planning and the self-invigilations are not touched -- the latter is now a
// predicate on the same table rather than a separate collection.
func (db *PG) ResetGeneratedInvigilations(ctx context.Context) error {
	if err := db.q(ctx).DeleteGeneratedInvigilations(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("cannot drop generated invigilations")
		return err
	}
	return nil
}

// RemovePrePlannedInvigilationAt deletes a pre-planned invigilation (key:
// starttime/roomName; roomName nil = the reserve). It reports whether a row was
// actually removed.
func (db *PG) RemovePrePlannedInvigilationAt(ctx context.Context, starttime time.Time, roomName *string) (bool, error) {
	n, err := db.q(ctx).DeletePrePlannedInvigilationAt(ctx, sqlc.DeletePrePlannedInvigilationAtParams{
		SemesterID: db.semesterID,
		Starttime:  starttime,
		RoomName:   roomName,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Interface("roomname", roomName).
			Msg("cannot delete pre planned invigilation")
		return false, err
	}
	return n > 0, nil
}

// GetInvigilatorRequirements returns what ZPA says about one invigilator, or
// (nil, nil) when ZPA delivered nothing about them.
func (db *PG) GetInvigilatorRequirements(ctx context.Context, teacherID int) (*zpa.SupervisorRequirements, error) {
	row, err := db.q(ctx).GetInvigilatorRequirement(ctx, sqlc.GetInvigilatorRequirementParams{
		SemesterID:    db.semesterID,
		InvigilatorID: teacherID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Int("invigilatorid", teacherID).
			Msg("cannot get requirements for inviglator")
		return nil, err
	}
	return requirementsFromRow(row), nil
}

// AllInvigilatorRequirements returns all stored ZPA invigilator requirements.
func (db *PG) AllInvigilatorRequirements(ctx context.Context) ([]*zpa.SupervisorRequirements, error) {
	rows, err := db.q(ctx).ListInvigilatorRequirements(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilator requirements")
		return nil, err
	}

	reqs := make([]*zpa.SupervisorRequirements, 0, len(rows))
	for _, row := range rows {
		reqs = append(reqs, requirementsFromRow(row))
	}
	return reqs, nil
}

// requirementsFromRow maps the stored ZPA requirements back onto the zpa type.
//
// excluded_dates stays a text[] of "02.01.06" strings because that is what ZPA
// delivers and what every reader parses; turning them into dates in the schema
// would move a parsing decision out of sight of the code that depends on it.
func requirementsFromRow(row sqlc.InvigilatorRequirement) *zpa.SupervisorRequirements {
	excluded := row.ExcludedDates
	if excluded == nil {
		excluded = make([]string, 0)
	}
	return &zpa.SupervisorRequirements{
		Invigilator:            row.Invigilator,
		InvigilatorID:          row.InvigilatorID,
		ExcludedDates:          excluded,
		PartTime:               row.PartTime,
		OralExamsContribution:  row.OralExamsContribution,
		LivecodingContribution: row.LivecodingContribution,
		MasterContribution:     row.MasterContribution,
		FreeSemester:           row.FreeSemester,
		OvertimeLastSemester:   row.OvertimeLastSemester,
		OvertimeThisSemester:   row.OvertimeThisSemester,
	}
}
