package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// SetSemester is the resolver for the setSemester field.
func (r *mutationResolver) SetSemester(ctx context.Context, input string) (*model.Semester, error) {
	return r.plexams.SetSemester(ctx, input)
}

// AllSemesterNames is the resolver for the allSemesterNames field.
func (r *queryResolver) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return r.plexams.GetAllSemesterNames(ctx)
}

// Semester is the resolver for the semester field.
func (r *queryResolver) Semester(ctx context.Context) (*model.Semester, error) {
	return r.plexams.GetSemester(ctx), nil
}

// Teachers is the resolver for the teachers field.
func (r *queryResolver) Teachers(ctx context.Context, fromZpa *bool) ([]*model.Teacher, error) {
	return r.plexams.GetTeachers(ctx, fromZpa)
}

// Invigilators is the resolver for the invigilators field.
func (r *queryResolver) Invigilators(ctx context.Context) ([]*model.Teacher, error) {
	return r.plexams.GetInvigilators(ctx)
}

// Zpaexams is the resolver for the zpaexams field.
func (r *queryResolver) Zpaexams(ctx context.Context, fromZpa *bool) ([]*model.ZPAExam, error) {
	return r.plexams.GetZPAExams(ctx, fromZpa)
}

// ZpaexamsByType is the resolver for the zpaexamsByType field.
func (r *queryResolver) ZpaexamsByType(ctx context.Context) ([]*model.ZPAExamsForType, error) {
	return r.plexams.GetZPAExamsGroupedByType(ctx)
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
