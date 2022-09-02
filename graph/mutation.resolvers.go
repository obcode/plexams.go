package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// SetSemester is the resolver for the setSemester field.
func (r *mutationResolver) SetSemester(ctx context.Context, input string) (*model.Semester, error) {
	return r.plexams.SetSemester(ctx, input)
}

// ZpaExamsToPlan is the resolver for the zpaExamsToPlan field.
func (r *mutationResolver) ZpaExamsToPlan(ctx context.Context, input []int) ([]*model.ZPAExam, error) {
	return r.plexams.ZpaExamsToPlan(ctx, input)
}

// AddZPAExamToPlan is the resolver for the addZPAExamToPlan field.
func (r *mutationResolver) AddZpaExamToPlan(ctx context.Context, anCode int) (bool, error) {
	return r.plexams.AddZpaExamToPlan(ctx, anCode)
}

// RmZPAExamFromPlan is the resolver for the rmZPAExamFromPlan field.
func (r *mutationResolver) RmZpaExamFromPlan(ctx context.Context, anCode int) (bool, error) {
	return r.plexams.RmZpaExamFromPlan(ctx, anCode)
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

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//   - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//     it when you're done.
//   - You have helper methods in this file. Move them out to keep these resolver files clean.
func (r *mutationResolver) DoneStep(ctx context.Context, number int) ([]*model.Step, error) {
	panic(fmt.Errorf("not implemented"))
}
