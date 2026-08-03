package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// BlockedRooms returns all room blocks of the semester, sorted by room/starttime.
func (db *PG) BlockedRooms(ctx context.Context) ([]*model.BlockedRoom, error) {
	rows, err := db.q(ctx).ListBlockedRooms(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get blocked rooms")
		return nil, err
	}

	blocked := make([]*model.BlockedRoom, 0, len(rows))
	for _, row := range rows {
		blocked = append(blocked, &model.BlockedRoom{
			Starttime: &row.Starttime,
			Room:      row.RoomName,
			Reason:    row.Reason,
		})
	}
	return blocked, nil
}

// BlockRoomForSlot stores (or updates) a room block (key: room + starttime). The
// block's Starttime must be set by the caller, from the slot's start time --
// under MongoDB a nil one produced a block nothing could address again.
func (db *PG) BlockRoomForSlot(ctx context.Context, block *model.BlockedRoom) error {
	if block.Starttime == nil {
		err := errNoStarttime
		log.Error().Err(err).Str("room", block.Room).Msg("cannot block room for slot")
		return err
	}

	err := db.q(ctx).UpsertBlockedRoom(ctx, sqlc.UpsertBlockedRoomParams{
		SemesterID: db.semesterID,
		RoomName:   block.Room,
		Starttime:  *block.Starttime,
		Reason:     block.Reason,
	})
	if err != nil {
		log.Error().Err(err).Str("room", block.Room).Msg("cannot block room for slot")
		return err
	}
	return nil
}

// UnblockRoomForSlot removes a room block (key: room + starttime). It reports
// whether a block was actually removed.
func (db *PG) UnblockRoomForSlot(ctx context.Context, room string, starttime time.Time) (bool, error) {
	n, err := db.q(ctx).DeleteBlockedRoom(ctx, sqlc.DeleteBlockedRoomParams{
		SemesterID: db.semesterID,
		RoomName:   room,
		Starttime:  starttime,
	})
	if err != nil {
		log.Error().Err(err).Str("room", room).Msg("cannot unblock room for slot")
		return false, err
	}
	return n > 0, nil
}
