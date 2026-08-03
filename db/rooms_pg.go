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

// roomFromRow maps a scanned row onto the GraphQL model.
//
// NeedsRequest is read, never written: it is a generated column. The Mongo data
// carries it under two keys (`needsRequest` and `needsrequest`, because
// model.Room has no bson tags), which is how the two spellings could drift --
// see the transfer notes on placesWithSocket/hmebSeats.
func roomFromRow(row sqlc.Room) *model.Room {
	return &model.Room{
		Name:             row.Name,
		Seats:            row.Seats,
		Handicap:         row.Handicap,
		Lab:              row.Lab,
		PlacesWithSocket: row.PlacesWithSocket,
		NeedsRequest:     row.NeedsRequest,
		RequestWith:      model.RoomRequestType(row.RequestWith),
		RequestPriority:  row.RequestPriority,
		Exahm:            row.Exahm,
		Seb:              row.Seb,
		SebSeats:         row.SebSeats,
		HmebSeats:        row.HmebSeats,
		Deactivated:      row.Deactivated,
		Hitzewert:        row.Hitzewert,
	}
}

func roomsFromRows(rows []sqlc.Room) []*model.Room {
	rooms := make([]*model.Room, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, roomFromRow(row))
	}
	return rooms
}

func (db *PG) RoomByName(ctx context.Context, roomName string) (*model.Room, error) {
	row, err := db.q(ctx).GetRoom(ctx, roomName)
	if errors.Is(err, pgx.ErrNoRows) {
		// Not (nil, nil): callers use the error as the "room does not exist"
		// signal (plexams.UpdateRoom, BlockRoomForSlot).
		return nil, fmt.Errorf("cannot find room %s", roomName)
	}
	if err != nil {
		log.Error().Err(err).Str("room", roomName).Msg("cannot find room")
		return nil, err
	}

	return roomFromRow(row), nil
}

// HasRoom reports whether a room with the given name exists.
func (db *PG) HasRoom(ctx context.Context, name string) (bool, error) {
	exists, err := db.q(ctx).RoomExists(ctx, name)
	if err != nil {
		log.Error().Err(err).Str("room", name).Msg("cannot check whether room exists")
		return false, err
	}
	return exists, nil
}

// AddRoom inserts a new room and returns it. Unlike the Mongo version this
// cannot create a second room of the same name: `rooms` had no unique index, so
// a caller that skipped the HasRoom check silently got a duplicate.
func (db *PG) AddRoom(ctx context.Context, room *model.Room) (*model.Room, error) {
	row, err := db.q(ctx).InsertRoom(ctx, sqlc.InsertRoomParams{
		Name:             room.Name,
		Seats:            room.Seats,
		Handicap:         room.Handicap,
		Lab:              room.Lab,
		PlacesWithSocket: room.PlacesWithSocket,
		RequestWith:      string(room.RequestWith),
		RequestPriority:  room.RequestPriority,
		Exahm:            room.Exahm,
		Seb:              room.Seb,
		SebSeats:         room.SebSeats,
		HmebSeats:        room.HmebSeats,
		Deactivated:      room.Deactivated,
		Hitzewert:        room.Hitzewert,
	})
	if err != nil {
		log.Error().Err(err).Str("room", room.Name).Msg("cannot insert room")
		return nil, err
	}

	return roomFromRow(row), nil
}

// ReplaceRoom replaces the room identified by its name (no upsert) and returns
// it. Errors if no room with that name exists.
func (db *PG) ReplaceRoom(ctx context.Context, room *model.Room) (*model.Room, error) {
	row, err := db.q(ctx).ReplaceRoom(ctx, sqlc.ReplaceRoomParams{
		Name:             room.Name,
		Seats:            room.Seats,
		Handicap:         room.Handicap,
		Lab:              room.Lab,
		PlacesWithSocket: room.PlacesWithSocket,
		RequestWith:      string(room.RequestWith),
		RequestPriority:  room.RequestPriority,
		Exahm:            room.Exahm,
		Seb:              room.Seb,
		SebSeats:         room.SebSeats,
		HmebSeats:        room.HmebSeats,
		Deactivated:      room.Deactivated,
		Hitzewert:        room.Hitzewert,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("cannot find room %s", room.Name)
	}
	if err != nil {
		log.Error().Err(err).Str("room", room.Name).Msg("cannot replace room")
		return nil, err
	}

	return roomFromRow(row), nil
}

// SetRoomDeactivated sets the deactivated flag of the room identified by name.
// Returns an error if no room with that name exists.
func (db *PG) SetRoomDeactivated(ctx context.Context, name string, deactivated bool) (*model.Room, error) {
	row, err := db.q(ctx).SetRoomDeactivated(ctx, sqlc.SetRoomDeactivatedParams{
		Name:        name,
		Deactivated: deactivated,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("cannot find room %s", name)
	}
	if err != nil {
		log.Error().Err(err).Str("room", name).Msg("cannot set room deactivated flag")
		return nil, err
	}

	return roomFromRow(row), nil
}

func (db *PG) Rooms(ctx context.Context) ([]*model.Room, error) {
	rows, err := db.q(ctx).ListRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get rooms")
		return nil, err
	}

	return roomsFromRows(rows), nil
}
