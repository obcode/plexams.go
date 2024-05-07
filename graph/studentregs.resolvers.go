package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.45

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// StudentByMtknr is the resolver for the studentByMtknr field.
func (r *queryResolver) StudentByMtknr(ctx context.Context, mtknr string) (*model.Student, error) {
	return r.plexams.StudentByMtknr(ctx, mtknr, nil)
}

// StudentsByName is the resolver for the studentsByName field.
func (r *queryResolver) StudentsByName(ctx context.Context, regex string) ([]*model.Student, error) {
	return r.plexams.StudentsByName(ctx, regex)
}

// Students is the resolver for the students field.
func (r *queryResolver) Students(ctx context.Context) ([]*model.Student, error) {
	return r.plexams.Students(ctx)
}
