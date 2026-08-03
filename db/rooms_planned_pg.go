package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// plannedRoomFromRow maps a row of planned_room_v onto the model.
//
// Starttime comes from the view -- that is, from the exam's plan entry -- and not
// from a column of its own. Under MongoDB it was stored alongside the room and
// nothing kept the two in step, so moving an exam left its rooms behind. See
// TestMovingAnExamCannotStaleTheRoomPlan.
func plannedRoomFromRow(row sqlc.ListPlannedRoomsRow) *model.PlannedRoom {
	students := row.Mtknrs
	if students == nil {
		students = make([]string, 0)
	}
	return &model.PlannedRoom{
		Starttime:         row.Starttime,
		RoomName:          row.RoomName,
		Ancode:            row.Ancode,
		Duration:          row.DurationMin,
		Handicap:          row.Handicap,
		HandicapRoomAlone: row.HandicapRoomAlone,
		Reserve:           row.Reserve,
		StudentsInRoom:    students,
		NtaMtknr:          row.NtaMtknr,
		PrePlanned:        row.PrePlanned,
	}
}

func (db *PG) PlannedRooms(ctx context.Context) ([]*model.PlannedRoom, error) {
	rows, err := db.q(ctx).ListPlannedRooms(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find planned rooms")
		return nil, err
	}

	rooms := make([]*model.PlannedRoom, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, plannedRoomFromRow(row))
	}
	return rooms, nil
}

func (db *PG) PlannedRoomsAt(ctx context.Context, starttime time.Time) ([]*model.PlannedRoom, error) {
	rows, err := db.q(ctx).ListPlannedRoomsAt(ctx, sqlc.ListPlannedRoomsAtParams{
		SemesterID: db.semesterID,
		Starttime:  &starttime,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Msg("cannot find rooms for slot")
		return nil, err
	}

	rooms := make([]*model.PlannedRoom, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, plannedRoomFromRow(sqlc.ListPlannedRoomsRow(row)))
	}
	return rooms, nil
}

func (db *PG) PlannedRoomsForAncode(ctx context.Context, ancode int) ([]*model.PlannedRoom, error) {
	rows, err := db.q(ctx).ListPlannedRoomsForAncode(ctx, sqlc.ListPlannedRoomsForAncodeParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot find rooms for ancode")
		return nil, err
	}

	rooms := make([]*model.PlannedRoom, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, plannedRoomFromRow(sqlc.ListPlannedRoomsRow(row)))
	}
	return rooms, nil
}

func (db *PG) PlannedRoomNames(ctx context.Context) ([]string, error) {
	names, err := db.q(ctx).ListPlannedRoomNames(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find distinct room names")
		return nil, err
	}
	return names, nil
}

func (db *PG) PlannedRoomNamesAt(ctx context.Context, starttime time.Time) ([]string, error) {
	names, err := db.q(ctx).ListPlannedRoomNamesAt(ctx, sqlc.ListPlannedRoomNamesAtParams{
		SemesterID: db.semesterID,
		Starttime:  &starttime,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Msg("cannot find roomnames for slot")
		return nil, err
	}
	return names, nil
}

// ResetPlannedRooms clears the generated room plan. The pre-planning is not
// touched.
func (db *PG) ResetPlannedRooms(ctx context.Context) error {
	if err := db.q(ctx).DeletePlannedRooms(ctx, db.semesterID); err != nil {
		log.Error().Err(err).Msg("cannot clear planned rooms")
		return err
	}
	return nil
}

// ReplacePlannedRooms swaps the whole room plan for a new one, in one
// transaction -- a failing insert used to leave the room plan empty.
//
// Two things the Mongo version could not do and this cannot avoid: a room for an
// exam that is not in the plan is rejected (the FK to plan_entry), and so is the
// same booking twice (the unique key). Both were previously found after the fact
// by plexams/validate_db.go.
func (db *PG) ReplacePlannedRooms(ctx context.Context, plannedRooms []*model.PlannedRoom) error {
	return db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeletePlannedRooms(ctx, db.semesterID); err != nil {
			log.Error().Err(err).Msg("cannot clear planned rooms")
			return err
		}

		for _, room := range plannedRooms {
			id, err := db.q(ctx).InsertPlannedRoom(ctx, sqlc.InsertPlannedRoomParams{
				SemesterID:        db.semesterID,
				Ancode:            room.Ancode,
				RoomName:          room.RoomName,
				DurationMin:       room.Duration,
				Handicap:          room.Handicap,
				HandicapRoomAlone: room.HandicapRoomAlone,
				Reserve:           room.Reserve,
				NtaMtknr:          room.NtaMtknr,
				PrePlanned:        room.PrePlanned,
			})
			if err != nil {
				log.Error().Err(err).Int("ancode", room.Ancode).Str("room", room.RoomName).
					Msg("cannot insert planned room")
				return err
			}

			for _, mtknr := range room.StudentsInRoom {
				if err := db.q(ctx).InsertPlannedRoomStudent(ctx, sqlc.InsertPlannedRoomStudentParams{
					PlannedRoomID: id,
					Mtknr:         mtknr,
				}); err != nil {
					log.Error().Err(err).Int("ancode", room.Ancode).Str("room", room.RoomName).
						Msg("cannot insert student into planned room")
					return err
				}
			}
		}
		return nil
	})
}

func prePlannedRoomFromRow(row sqlc.PrePlannedRoom) *model.PrePlannedRoom {
	return &model.PrePlannedRoom{
		Ancode:   row.Ancode,
		RoomName: row.RoomName,
		Mtknr:    row.Mtknr,
		Reserve:  row.Reserve,
		Seats:    row.Seats,
	}
}

func prePlannedRoomsFromRows(rows []sqlc.PrePlannedRoom) []*model.PrePlannedRoom {
	rooms := make([]*model.PrePlannedRoom, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, prePlannedRoomFromRow(row))
	}
	return rooms
}

func (db *PG) PrePlannedRooms(ctx context.Context) ([]*model.PrePlannedRoom, error) {
	rows, err := db.q(ctx).ListPrePlannedRooms(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre planned rooms")
		return nil, err
	}
	return prePlannedRoomsFromRows(rows), nil
}

func (db *PG) PrePlannedRoomsForExam(ctx context.Context, ancode int) ([]*model.PrePlannedRoom, error) {
	rows, err := db.q(ctx).ListPrePlannedRoomsForExam(ctx, sqlc.ListPrePlannedRoomsForExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get pre planned rooms")
		return nil, err
	}
	return prePlannedRoomsFromRows(rows), nil
}

// AddPrePlannedRoomToExam pins a room for an exam, replacing an existing pin for
// the same (ancode, room, student). The bool return is what it always was: true
// unless the write failed.
func (db *PG) AddPrePlannedRoomToExam(ctx context.Context, prePlannedRoom *model.PrePlannedRoom) (bool, error) {
	err := db.q(ctx).UpsertPrePlannedRoom(ctx, sqlc.UpsertPrePlannedRoomParams{
		SemesterID: db.semesterID,
		Ancode:     prePlannedRoom.Ancode,
		RoomName:   prePlannedRoom.RoomName,
		Mtknr:      prePlannedRoom.Mtknr,
		Reserve:    prePlannedRoom.Reserve,
		Seats:      prePlannedRoom.Seats,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", prePlannedRoom.Ancode).
			Str("roomname", prePlannedRoom.RoomName).Interface("mtknr", prePlannedRoom.Mtknr).
			Msg("cannot insert pre planned room")
		return false, err
	}
	return true, nil
}

// RemovePrePlannedRoomFromExam deletes a pre-planned room from an exam (key:
// ancode/roomName/mtknr). It reports whether a row was actually removed.
func (db *PG) RemovePrePlannedRoomFromExam(ctx context.Context, ancode int, roomName string, mtknr *string) (bool, error) {
	n, err := db.q(ctx).DeletePrePlannedRoom(ctx, sqlc.DeletePrePlannedRoomParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
		RoomName:   roomName,
		Mtknr:      mtknr,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Str("roomname", roomName).
			Interface("mtknr", mtknr).Msg("cannot delete pre planned room")
		return false, err
	}
	return n > 0, nil
}
