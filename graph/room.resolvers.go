package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// Room is the resolver for the room field.
func (r *plannedRoomResolver) Room(ctx context.Context, obj *model.PlannedRoom) (*model.Room, error) {
	return r.plexams.RoomFromName(ctx, obj.RoomName)
}

// Rooms is the resolver for the rooms field.
func (r *queryResolver) Rooms(ctx context.Context) ([]*model.Room, error) {
	return r.plexams.Rooms(ctx)
}

// PlannedRoomNames is the resolver for the plannedRoomNames field.
func (r *queryResolver) PlannedRoomNames(ctx context.Context) ([]string, error) {
	return r.plexams.PlannedRoomNames(ctx)
}

// PlannedRoomNamesInSlot is the resolver for the plannedRoomNamesInSlot field.
func (r *queryResolver) PlannedRoomNamesInSlot(ctx context.Context, day int, time int) ([]string, error) {
	return r.plexams.PlannedRoomNamesInSlot(ctx, day, time)
}

// PlannedRoomsInSlot is the resolver for the plannedRoomsInSlot field.
func (r *queryResolver) PlannedRoomsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedRoom, error) {
	return r.plexams.PlannedRoomsInSlot(ctx, day, time)
}

// Room is the resolver for the room field.
func (r *roomForExamResolver) Room(ctx context.Context, obj *model.RoomForExam) (*model.Room, error) {
	return r.plexams.Room(ctx, obj)
}

// PlannedRoom returns generated.PlannedRoomResolver implementation.
func (r *Resolver) PlannedRoom() generated.PlannedRoomResolver { return &plannedRoomResolver{r} }

// RoomForExam returns generated.RoomForExamResolver implementation.
func (r *Resolver) RoomForExam() generated.RoomForExamResolver { return &roomForExamResolver{r} }

type plannedRoomResolver struct{ *Resolver }
type roomForExamResolver struct{ *Resolver }
