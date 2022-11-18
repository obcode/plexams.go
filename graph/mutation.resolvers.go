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

// ZpaExamsToPlan is the resolver for the zpaExamsToPlan field.
func (r *mutationResolver) ZpaExamsToPlan(ctx context.Context, input []int) ([]*model.ZPAExam, error) {
	return r.plexams.ZpaExamsToPlan(ctx, input)
}

// AddZPAExamToPlan is the resolver for the addZPAExamToPlan field.
func (r *mutationResolver) AddZpaExamToPlan(ctx context.Context, ancode int, unknown bool) (bool, error) {
	return r.plexams.AddZpaExamToPlan(ctx, ancode, unknown)
}

// RmZPAExamFromPlan is the resolver for the rmZPAExamFromPlan field.
func (r *mutationResolver) RmZpaExamFromPlan(ctx context.Context, ancode int, unknown bool) (bool, error) {
	return r.plexams.RmZpaExamFromPlan(ctx, ancode, unknown)
}

// AddAdditionalExam is the resolver for the addAdditionalExam field.
func (r *mutationResolver) AddAdditionalExam(ctx context.Context, exam model.AdditionalExamInput) (bool, error) {
	return r.plexams.AddAdditionalExam(ctx, exam)
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

// NotPlannedByMe is the resolver for the notPlannedByMe field.
func (r *mutationResolver) NotPlannedByMe(ctx context.Context, ancode int) (bool, error) {
	return r.plexams.NotPlannedByMe(ctx, ancode)
}

// ExcludeDays is the resolver for the excludeDays field.
func (r *mutationResolver) ExcludeDays(ctx context.Context, ancode int, days []string) (bool, error) {
	return r.plexams.ExcludeDays(ctx, ancode, days)
}

// PossibleDays is the resolver for the possibleDays field.
func (r *mutationResolver) PossibleDays(ctx context.Context, ancode int, days []string) (bool, error) {
	return r.plexams.PossibleDays(ctx, ancode, days)
}

// SameSlot is the resolver for the sameSlot field.
func (r *mutationResolver) SameSlot(ctx context.Context, ancode int, ancodes []int) (bool, error) {
	return r.plexams.SameSlot(ctx, ancode, ancodes)
}

// RoomWithSockets is the resolver for the roomWithSockets field.
func (r *mutationResolver) PlacesWithSockets(ctx context.Context, ancode int) (bool, error) {
	return r.plexams.PlacesWithSockets(ctx, ancode)
}

// Lab is the resolver for the lab field.
func (r *mutationResolver) Lab(ctx context.Context, ancode int) (bool, error) {
	return r.plexams.Lab(ctx, ancode)
}

// ExahmRooms is the resolver for the exahmRooms field.
func (r *mutationResolver) ExahmRooms(ctx context.Context, ancode int) (bool, error) {
	return r.plexams.ExahmRooms(ctx, ancode)
}

// Online is the resolver for the online field.
func (r *mutationResolver) Online(ctx context.Context, ancode int) (bool, error) {
	return r.plexams.Online(ctx, ancode)
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

type mutationResolver struct{ *Resolver }
