package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// InvigilatorTodos is the resolver for the invigilatorTodos field.
func (r *queryResolver) InvigilatorTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	return r.plexams.InvigilationTodos(ctx)
}