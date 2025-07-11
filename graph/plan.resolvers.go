package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.75

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/generated"
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

// Starttime is the resolver for the starttime field.
func (r *planEntryResolver) Starttime(ctx context.Context, obj *model.PlanEntry) (*time.Time, error) {
	return r.plexams.GetStarttime(obj.DayNumber, obj.SlotNumber)
}

// AllProgramsInPlan is the resolver for the allProgramsInPlan field.
func (r *queryResolver) AllProgramsInPlan(ctx context.Context) ([]string, error) {
	return r.plexams.AllProgramsInPlan(ctx)
}

// AncodesInPlan is the resolver for the ancodesInPlan field.
func (r *queryResolver) AncodesInPlan(ctx context.Context) ([]int, error) {
	return r.plexams.AncodesInPlan(ctx)
}

// ExamerNamesInPlan is the resolver for the examerNamesInPlan field.
func (r *queryResolver) ExamerInPlan(ctx context.Context) ([]*model.ExamerInPlan, error) {
	return r.plexams.ExamerInPlan(ctx)
}

// PreExamsInSlot is the resolver for the preExamsInSlot field.
func (r *queryResolver) PreExamsInSlot(ctx context.Context, day int, time int) ([]*model.PreExam, error) {
	return r.plexams.PreExamsInSlot(ctx, day, time)
}

// ExamGroupsInSlot is the resolver for the examGroupsInSlot field.
func (r *queryResolver) ExamsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedExam, error) {
	return r.plexams.ExamsInSlot(ctx, day, time)
}

// ExamsWithoutSlot is the resolver for the examsWithoutSlot field.
func (r *queryResolver) ExamsWithoutSlot(ctx context.Context) ([]*model.PlannedExam, error) {
	return r.plexams.ExamsWithoutSlot(ctx)
}

// AllowedSlots is the resolver for the allowedSlots field.
func (r *queryResolver) AllowedSlots(ctx context.Context, ancode int) ([]*model.Slot, error) {
	return r.plexams.AllowedSlots(ctx, ancode)
}

// AwkwardSlots is the resolver for the awkwardSlots field.
func (r *queryResolver) AwkwardSlots(ctx context.Context, ancode int) ([]*model.Slot, error) {
	return r.plexams.AwkwardSlots(ctx, ancode)
}

// PlanEntry returns generated.PlanEntryResolver implementation.
func (r *Resolver) PlanEntry() generated.PlanEntryResolver { return &planEntryResolver{r} }

type planEntryResolver struct{ *Resolver }
