package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.30

import (
	"context"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// Room is the resolver for the room field.
func (r *roomForExamResolver) Room(ctx context.Context, obj *model.RoomForExam) (*model.Room, error) {
	return r.plexams.Room(ctx, obj)
}

// RoomForExam returns generated.RoomForExamResolver implementation.
func (r *Resolver) RoomForExam() generated.RoomForExamResolver { return &roomForExamResolver{r} }

type roomForExamResolver struct{ *Resolver }
