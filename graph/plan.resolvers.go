package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

// AddExamToSlot is the resolver for the addExamToSlot field.
func (r *mutationResolver) AddExamToSlot(ctx context.Context, day int, time int, ancode int) (bool, error) {
	panic(fmt.Errorf("not implemented: AddExamToSlot - addExamToSlot"))
}

// RmExamFromSlot is the resolver for the rmExamFromSlot field.
func (r *mutationResolver) RmExamFromSlot(ctx context.Context, ancode int) (bool, error) {
	panic(fmt.Errorf("not implemented: RmExamFromSlot - rmExamFromSlot"))
}

// ExamsWithoutSlot is the resolver for the examsWithoutSlot field.
func (r *queryResolver) ExamsWithoutSlot(ctx context.Context) ([]*model.GeneratedExam, error) {
	return r.plexams.ExamsWithoutSlot(ctx)
}
