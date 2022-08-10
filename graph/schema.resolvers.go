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

// StudentRegs is the resolver for the studentRegs field.
func (r *primussExamResolver) StudentRegs(ctx context.Context, obj *model.PrimussExam) ([]*model.StudentReg, error) {
	return r.plexams.GetStudentRegs(ctx, obj)
}

// Conflicts is the resolver for the conflicts field.
func (r *primussExamResolver) Conflicts(ctx context.Context, obj *model.PrimussExam) (*model.Conflicts, error) {
	return r.plexams.GetConflicts(ctx, obj)
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

// ZpaExams is the resolver for the zpaExams field.
func (r *queryResolver) ZpaExams(ctx context.Context, fromZpa *bool) ([]*model.ZPAExam, error) {
	return r.plexams.GetZPAExams(ctx, fromZpa)
}

// ZpaExamsByType is the resolver for the zpaExamsByType field.
func (r *queryResolver) ZpaExamsByType(ctx context.Context) ([]*model.ZPAExamsForType, error) {
	return r.plexams.GetZPAExamsGroupedByType(ctx)
}

// ZpaExam is the resolver for the zpaExam field.
func (r *queryResolver) ZpaExam(ctx context.Context, anCode int) (*model.ZPAExam, error) {
	return r.plexams.GetZpaExamByAncode(ctx, anCode)
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

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// PrimussExam returns generated.PrimussExamResolver implementation.
func (r *Resolver) PrimussExam() generated.PrimussExamResolver { return &primussExamResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type primussExamResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
