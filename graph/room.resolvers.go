package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.75

import (
	"context"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// PrePlanRoom is the resolver for the prePlanRoom field.
func (r *mutationResolver) PrePlanRoom(ctx context.Context, ancode int, roomName string, reserve bool, mtknr *string) (bool, error) {
	return r.plexams.PreAddRoomToExam(ctx, ancode, roomName, mtknr, reserve)
}

// Room is the resolver for the room field.
func (r *plannedRoomResolver) Room(ctx context.Context, obj *model.PlannedRoom) (*model.Room, error) {
	return r.plexams.RoomByName(ctx, obj.RoomName)
}

// Rooms is the resolver for the rooms field.
func (r *queryResolver) Rooms(ctx context.Context) ([]*model.Room, error) {
	return r.plexams.Rooms(ctx)
}

// RoomsForSlots is the resolver for the roomsForSlots field.
func (r *queryResolver) RoomsForSlots(ctx context.Context) ([]*model.RoomsForSlot, error) {
	return r.plexams.RoomsForSlots(ctx)
}

// PlannedRooms is the resolver for the plannedRooms field.
func (r *queryResolver) PlannedRooms(ctx context.Context) ([]*model.PlannedRoom, error) {
	return r.plexams.PlannedRooms(ctx)
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

// PlannedRoomForNta is the resolver for the plannedRoomForNTA field.
func (r *queryResolver) PlannedRoomForStudent(ctx context.Context, ancode int, mtknr string) (*model.PlannedRoom, error) {
	return r.plexams.PlannedRoomForStudent(ctx, ancode, mtknr)
}

// Rooms is the resolver for the rooms field.
func (r *roomsForSlotResolver) Rooms(ctx context.Context, obj *model.RoomsForSlot) ([]*model.Room, error) {
	return r.plexams.RoomsFromRoomNames(ctx, obj.RoomNames)
}

// PlannedRoom returns generated.PlannedRoomResolver implementation.
func (r *Resolver) PlannedRoom() generated.PlannedRoomResolver { return &plannedRoomResolver{r} }

// RoomsForSlot returns generated.RoomsForSlotResolver implementation.
func (r *Resolver) RoomsForSlot() generated.RoomsForSlotResolver { return &roomsForSlotResolver{r} }

type plannedRoomResolver struct{ *Resolver }
type roomsForSlotResolver struct{ *Resolver }
