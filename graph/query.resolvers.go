package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

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

// Teacher is the resolver for the teacher field.
func (r *queryResolver) Teacher(ctx context.Context, id int) (*model.Teacher, error) {
	return r.plexams.GetTeacher(ctx, id)
}

// Teachers is the resolver for the teachers field.
func (r *queryResolver) Teachers(ctx context.Context, fromZpa *bool) ([]*model.Teacher, error) {
	return r.plexams.GetTeachers(ctx, fromZpa)
}

// Invigilators is the resolver for the invigilators field.
func (r *queryResolver) Invigilators(ctx context.Context) ([]*model.Teacher, error) {
	return r.plexams.GetInvigilators(ctx)
}

// Fk07programs is the resolver for the fk07programs field.
func (r *queryResolver) Fk07programs(ctx context.Context) ([]*model.FK07Program, error) {
	return r.plexams.GetFk07programs(ctx)
}

// ZpaExams is the resolver for the zpaExams field.
func (r *queryResolver) ZpaExams(ctx context.Context, fromZpa *bool) ([]*model.ZPAExam, error) {
	return r.plexams.GetZPAExams(ctx, fromZpa)
}

// ZpaExamsByType is the resolver for the zpaExamsByType field.
func (r *queryResolver) ZpaExamsByType(ctx context.Context) ([]*model.ZPAExamsForType, error) {
	return r.plexams.GetZPAExamsGroupedByType(ctx)
}

// ZpaExamsToPlan is the resolver for the zpaExamsToPlan field.
func (r *queryResolver) ZpaExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return r.plexams.GetZpaExamsToPlan(ctx)
}

// ZpaExamsNotToPlan is the resolver for the zpaExamsNotToPlan field.
func (r *queryResolver) ZpaExamsNotToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return r.plexams.GetZpaExamsNotToPlan(ctx)
}

// ZpaExamsPlaningStatusUnknown is the resolver for the zpaExamsPlaningStatusUnknown field.
func (r *queryResolver) ZpaExamsPlaningStatusUnknown(ctx context.Context) ([]*model.ZPAExam, error) {
	return r.plexams.ZpaExamsPlaningStatusUnknown(ctx)
}

// ZpaExam is the resolver for the zpaExam field.
func (r *queryResolver) ZpaExam(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	return r.plexams.GetZpaExamByAncode(ctx, ancode)
}

// ZpaAnCodes is the resolver for the zpaAnCodes field.
func (r *queryResolver) ZpaAnCodes(ctx context.Context) ([]*model.AnCode, error) {
	return r.plexams.GetZpaAnCodes(ctx)
}

// StudentRegsImportErrors is the resolver for the studentRegsImportErrors field.
func (r *queryResolver) StudentRegsImportErrors(ctx context.Context) ([]*model.RegWithError, error) {
	return r.plexams.StudentRegsImportErrors(ctx)
}

// AdditionalExams is the resolver for the additionalExams field.
func (r *queryResolver) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	return r.plexams.AdditionalExams(ctx)
}

// PrimussExams is the resolver for the primussExams field.
func (r *queryResolver) PrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	return r.plexams.PrimussExams(ctx)
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

// ConnectedExam is the resolver for the connectedExam field.
func (r *queryResolver) ConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	return r.plexams.GetConnectedExam(ctx, ancode)
}

// ConnectedExams is the resolver for the connectedExams field.
func (r *queryResolver) ConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	return r.plexams.GetConnectedExams(ctx)
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

// ConflictingGroupCodes is the resolver for the conflictingGroupCodes field.
func (r *queryResolver) ConflictingGroupCodes(ctx context.Context, examGroupCode int) ([]*model.ExamGroupConflict, error) {
	return r.plexams.ConflictingGroupCodes(ctx, examGroupCode)
}

// Ntas is the resolver for the ntas field.
func (r *queryResolver) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return r.plexams.Ntas(ctx)
}

// NtasWithRegs is the resolver for the ntasWithRegs field.
func (r *queryResolver) NtasWithRegs(ctx context.Context) ([]*model.NTAWithRegs, error) {
	return r.plexams.NtasWithRegs(ctx)
}

// NtasWithRegsByTeacher is the resolver for the ntasWithRegsByTeacher field.
func (r *queryResolver) NtasWithRegsByTeacher(ctx context.Context) ([]*model.NTAWithRegsByExamAndTeacher, error) {
	return r.plexams.NtasWithRegsByTeacher(ctx)
}

// Nta is the resolver for the nta field.
func (r *queryResolver) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return r.plexams.Nta(ctx, mtknr)
}

// AllowedSlots is the resolver for the allowedSlots field.
func (r *queryResolver) AllowedSlots(ctx context.Context, examGroupCode int) ([]*model.Slot, error) {
	return r.plexams.AllowedSlots(ctx, examGroupCode)
}

// AwkwardSlots is the resolver for the awkwardSlots field.
func (r *queryResolver) AwkwardSlots(ctx context.Context, examGroupCode int) ([]*model.Slot, error) {
	return r.plexams.AwkwardSlots(ctx, examGroupCode)
}

// ExamGroupsInSlot is the resolver for the examGroupsInSlot field.
func (r *queryResolver) ExamGroupsInSlot(ctx context.Context, day int, time int) ([]*model.ExamGroup, error) {
	return r.plexams.ExamGroupsInSlot(ctx, day, time)
}

// ExamGroupsWithoutSlot is the resolver for the examGroupsWithoutSlot field.
func (r *queryResolver) ExamGroupsWithoutSlot(ctx context.Context) ([]*model.ExamGroup, error) {
	return r.plexams.ExamGroupsWithoutSlot(ctx)
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

// PlannedExamsInSlot is the resolver for the plannedExamsInSlot field.
func (r *queryResolver) PlannedExamsInSlot(ctx context.Context, day int, time int) ([]*model.PlannedExamWithNta, error) {
	return r.plexams.PlannedExamsInSlot(ctx, day, time)
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
