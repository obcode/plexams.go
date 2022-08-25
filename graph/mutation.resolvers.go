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

// RemovePrimussExam is the resolver for the removePrimussExam field.
func (r *mutationResolver) RemovePrimussExam(ctx context.Context, input *model.PrimussExamInput) (bool, error) {
	return r.plexams.RemovePrimussExam(ctx, input)
}

// PrepareExams is the resolver for the prepareExams field.
func (r *mutationResolver) PrepareExams(ctx context.Context, input []*model.PrimussExamInput) (bool, error) {
	return r.plexams.PrepareExams(ctx, input)
}

// AddNta is the resolver for the addNTA field.
func (r *mutationResolver) AddNta(ctx context.Context, input model.NTAInput) (*model.NTA, error) {
	return r.plexams.AddNta(ctx, input)
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

type mutationResolver struct{ *Resolver }