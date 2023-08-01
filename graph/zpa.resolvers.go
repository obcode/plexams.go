package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// Teacher is the resolver for the teacher field.
func (r *queryResolver) Teacher(ctx context.Context, id int) (*model.Teacher, error) {
	return r.plexams.GetTeacher(ctx, id)
}

// Teachers is the resolver for the teachers field.
func (r *queryResolver) Teachers(ctx context.Context, fromZpa *bool) ([]*model.Teacher, error) {
	return r.plexams.GetTeachers(ctx, fromZpa)
}

// Invigilators is the resolver for the invigilators field.
func (r *queryResolver) Invigilators(ctx context.Context) ([]*model.ZPAInvigilator, error) {
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
