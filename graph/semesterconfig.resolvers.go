package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.75

import (
	"context"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// AllSemesterNames is the resolver for the allSemesterNames field.
func (r *queryResolver) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return r.plexams.GetAllSemesterNames(ctx)
}

// Semester is the resolver for the semester field.
func (r *queryResolver) Semester(ctx context.Context) (*model.Semester, error) {
	return r.plexams.GetSemester(ctx), nil
}

// SemesterConfig is the resolver for the SemesterConfig field.
func (r *queryResolver) SemesterConfig(ctx context.Context) (*model.SemesterConfig, error) {
	return r.plexams.GetSemesterConfig(), nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
