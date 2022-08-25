package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// InitWorkflow is the resolver for the initWorkflow field.
func (r *mutationResolver) InitWorkflow(ctx context.Context) ([]*model.Step, error) {
	return r.plexams.InitWorkflow(ctx)
}

// DoneStep is the resolver for the doneStep field.
func (r *mutationResolver) DoneStep(ctx context.Context, number int) ([]*model.Step, error) {
	panic(fmt.Errorf("not implemented"))
}

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
