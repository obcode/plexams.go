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

// errNoStarttime is what a room request or room block without a start time gets
// instead of a row nothing can address again. Under MongoDB it was stored, and
// the request could then be neither approved nor deleted.
var errNoStarttime = errors.New("no starttime given")

func roomRequestFromRow(row sqlc.RoomRequest) *model.RoomRequest {
	starttime := row.Starttime
	return &model.RoomRequest{
		Room:      row.RoomName,
		Starttime: &starttime,
		From:      row.ValidFrom,
		Until:     row.ValidUntil,
		Approved:  row.Approved,
		Active:    row.Active,
	}
}

// RoomRequests returns all building-management room requests of the semester,
// sorted by room, starttime.
func (db *PG) RoomRequests(ctx context.Context) ([]*model.RoomRequest, error) {
	rows, err := db.q(ctx).ListRoomRequests(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get room requests")
		return nil, err
	}

	requests := make([]*model.RoomRequest, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, roomRequestFromRow(row))
	}
	return requests, nil
}

// ReplaceAllRoomRequests replaces the semester's room requests, in one
// transaction. The caller has already refused to discard approved requests
// without force (plexams/room_requests.go:62).
func (db *PG) ReplaceAllRoomRequests(ctx context.Context, requests []*model.RoomRequest) error {
	return db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteRoomRequests(ctx, db.semesterID); err != nil {
			log.Error().Err(err).Msg("cannot drop room requests")
			return err
		}
		for _, request := range requests {
			if err := db.insertRoomRequest(ctx, request); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddRoomRequest inserts a single room request.
func (db *PG) AddRoomRequest(ctx context.Context, request *model.RoomRequest) error {
	return db.insertRoomRequest(ctx, request)
}

func (db *PG) insertRoomRequest(ctx context.Context, request *model.RoomRequest) error {
	if request.Starttime == nil {
		log.Error().Err(errNoStarttime).Str("room", request.Room).Msg("cannot insert room request")
		return errNoStarttime
	}

	err := db.q(ctx).InsertRoomRequest(ctx, sqlc.InsertRoomRequestParams{
		SemesterID: db.semesterID,
		RoomName:   request.Room,
		Starttime:  *request.Starttime,
		ValidFrom:  request.From,
		ValidUntil: request.Until,
		Approved:   request.Approved,
		Active:     request.Active,
	})
	if err != nil {
		log.Error().Err(err).Str("room", request.Room).Msg("cannot insert room request")
		return err
	}
	return nil
}

// GetRoomRequest returns one room request (key: room/starttime) or (nil, nil).
// plexams.AddRoomRequest reads the nil as "does not exist yet".
func (db *PG) GetRoomRequest(ctx context.Context, room string, starttime time.Time) (*model.RoomRequest, error) {
	row, err := db.q(ctx).GetRoomRequest(ctx, sqlc.GetRoomRequestParams{
		SemesterID: db.semesterID,
		RoomName:   room,
		Starttime:  starttime,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot get room request")
		return nil, err
	}
	return roomRequestFromRow(row), nil
}

// SetRoomRequestApproved sets the approved flag (key: room/starttime). Returns
// nil if no such request exists.
func (db *PG) SetRoomRequestApproved(ctx context.Context, room string, starttime time.Time, approved bool) (*model.RoomRequest, error) {
	row, err := db.q(ctx).SetRoomRequestApproved(ctx, sqlc.SetRoomRequestApprovedParams{
		SemesterID: db.semesterID,
		RoomName:   room,
		Starttime:  starttime,
		Approved:   approved,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot update room request")
		return nil, err
	}
	return roomRequestFromRow(row), nil
}

// SetRoomRequestActive sets the active flag (key: room/starttime). Returns nil if
// no such request exists.
func (db *PG) SetRoomRequestActive(ctx context.Context, room string, starttime time.Time, active bool) (*model.RoomRequest, error) {
	row, err := db.q(ctx).SetRoomRequestActive(ctx, sqlc.SetRoomRequestActiveParams{
		SemesterID: db.semesterID,
		RoomName:   room,
		Starttime:  starttime,
		Active:     active,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot update room request")
		return nil, err
	}
	return roomRequestFromRow(row), nil
}

// UpdateRoomRequestTime changes the time range of a room request (key:
// room/starttime). Returns nil if no such request exists.
//
// It changes valid_from, which is why that column must not be part of the key --
// otherwise this would move the row out from under the caller that just edited it.
func (db *PG) UpdateRoomRequestTime(ctx context.Context, room string, starttime time.Time, from, until time.Time) (*model.RoomRequest, error) {
	row, err := db.q(ctx).SetRoomRequestWindow(ctx, sqlc.SetRoomRequestWindowParams{
		SemesterID: db.semesterID,
		RoomName:   room,
		Starttime:  starttime,
		ValidFrom:  from,
		ValidUntil: until,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot update room request time")
		return nil, err
	}
	return roomRequestFromRow(row), nil
}
