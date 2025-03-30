package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.70

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// Workflow is the resolver for the workflow field.
func (r *queryResolver) Workflow(ctx context.Context) ([]*model.Step, error) {
	return r.plexams.GetWorkflow(ctx)
}

// NextDeadline is the resolver for the nextDeadline field.
func (r *queryResolver) NextDeadline(ctx context.Context) (*model.Step, error) {
	return r.plexams.NextDeadline(ctx)
}

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

// AdditionalExams is the resolver for the additionalExams field.
func (r *queryResolver) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	return r.plexams.AdditionalExams(ctx)
}

// PrimussExam is the resolver for the primussExam field.
func (r *queryResolver) PrimussExam(ctx context.Context, program string, ancode int) (*model.PrimussExam, error) {
	return r.plexams.GetPrimussExam(ctx, program, ancode)
}

// PrimussExamsForAnCode is the resolver for the primussExamsForAnCode field.
func (r *queryResolver) PrimussExamsForAnCode(ctx context.Context, ancode int) ([]*model.PrimussExam, error) {
	return r.plexams.GetPrimussExamsForAncode(ctx, ancode)
}

// StudentRegsForProgram is the resolver for the studentRegsForProgram field.
func (r *queryResolver) StudentRegsForProgram(ctx context.Context, program string) ([]*model.StudentReg, error) {
	return r.plexams.StudentRegsForProgram(ctx, program)
}

// ExamWithRegs is the resolver for the examWithRegs field.
func (r *queryResolver) ExamWithRegs(ctx context.Context, ancode int) (*model.ExamWithRegs, error) {
	return r.plexams.ExamWithRegs(ctx, ancode)
}

// ExamsWithRegs is the resolver for the examsWithRegs field.
func (r *queryResolver) ExamsWithRegs(ctx context.Context) ([]*model.ExamWithRegs, error) {
	return r.plexams.ExamsWithRegs(ctx)
}

// ConstraintForAncode is the resolver for the constraintForAncode field.
func (r *queryResolver) ConstraintForAncode(ctx context.Context, ancode int) (*model.Constraints, error) {
	return r.plexams.ConstraintForAncode(ctx, ancode)
}

// ZpaExamsToPlanWithConstraints is the resolver for the zpaExamsToPlanWithConstraints field.
func (r *queryResolver) ZpaExamsToPlanWithConstraints(ctx context.Context) ([]*model.ZPAExamWithConstraints, error) {
	return r.plexams.ZpaExamsToPlanWithConstraints(ctx)
}

// ExamGroups is the resolver for the examGroups field.
func (r *queryResolver) ExamGroups(ctx context.Context) ([]*model.ExamGroup, error) {
	return r.plexams.ExamGroups(ctx)
}

// ExamGroup is the resolver for the examGroup field.
func (r *queryResolver) ExamGroup(ctx context.Context, examGroupCode int) (*model.ExamGroup, error) {
	return r.plexams.ExamGroup(ctx, examGroupCode)
}

// NtasWithRegsByTeacher is the resolver for the ntasWithRegsByTeacher field.
func (r *queryResolver) NtasWithRegsByTeacher(ctx context.Context) ([]*model.NTAWithRegsByExamAndTeacher, error) {
	return r.plexams.NtasWithRegsByTeacher(ctx)
}

// Nta is the resolver for the nta field.
func (r *queryResolver) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return r.plexams.Nta(ctx, mtknr)
}

// ExamGroupsWithoutSlot is the resolver for the examGroupsWithoutSlot field.
func (r *queryResolver) ExamGroupsWithoutSlot(ctx context.Context) ([]*model.ExamGroup, error) {
	return r.plexams.ExamGroupsWithoutSlot(ctx)
}

// PlannedExamsInSlot is the resolver for the plannedExamsInSlot field.
func (r *queryResolver) PlannedExamsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedExamWithNta, error) {
	return nil, nil
}

// ExamsInPlan is the resolver for the examsInPlan field.
func (r *queryResolver) ExamsInPlan(ctx context.Context) ([]*model.ExamInPlan, error) {
	return r.plexams.ExamsInPlan(ctx)
}

// ExamsInSlotWithRooms is the resolver for the examsInSlotWithRooms field.
func (r *queryResolver) ExamsInSlotWithRooms(ctx context.Context, day int, time int) ([]*model.ExamWithRegsAndRooms, error) {
	return r.plexams.ExamsInSlotWithRooms(ctx, day, time)
}

// RoomsWithConstraints is the resolver for the roomsWithConstraints field.
func (r *queryResolver) RoomsWithConstraints(ctx context.Context, handicap bool, lab bool, placesWithSocket bool, exahm *bool) ([]*model.Room, error) {
	panic(fmt.Errorf("not implemented: RoomsWithConstraints - roomsWithConstraints"))
}

// RoomsForSlot is the resolver for the roomsForSlot field.
func (r *queryResolver) RoomsForSlot(ctx context.Context, day int, time int) (*model.SlotWithRooms, error) {
	return r.plexams.RoomsForSlot(ctx, day, time)
}

// DayOkForInvigilator is the resolver for the dayOkForInvigilator field.
func (r *queryResolver) DayOkForInvigilator(ctx context.Context, day int, invigilatorID int) (*bool, error) {
	panic(fmt.Errorf("not implemented: DayOkForInvigilator - dayOkForInvigilator"))
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
