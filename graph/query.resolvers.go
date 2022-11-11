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
func (r *queryResolver) ZpaExam(ctx context.Context, anCode int) (*model.ZPAExam, error) {
	return r.plexams.GetZpaExamByAncode(ctx, anCode)
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
func (r *queryResolver) PrimussExam(ctx context.Context, program string, anCode int) (*model.PrimussExam, error) {
	return r.plexams.GetPrimussExam(ctx, program, anCode)
}

// PrimussExamsForAnCode is the resolver for the primussExamsForAnCode field.
func (r *queryResolver) PrimussExamsForAnCode(ctx context.Context, anCode int) ([]*model.PrimussExam, error) {
	return r.plexams.GetPrimussExamsForAncode(ctx, anCode)
}

// StudentRegsForProgram is the resolver for the studentRegsForProgram field.
func (r *queryResolver) StudentRegsForProgram(ctx context.Context, program string) ([]*model.StudentReg, error) {
	return r.plexams.StudentRegsForProgram(ctx, program)
}

// ConnectedExams is the resolver for the connectedExams field.
func (r *queryResolver) ConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	return r.plexams.GetConnectedExams(ctx)
}

// Ntas is the resolver for the ntas field.
func (r *queryResolver) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return r.plexams.Ntas(ctx)
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
