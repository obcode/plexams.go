package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
)

// StudentRegs is the resolver for the studentRegs field.
func (r *primussExamResolver) StudentRegs(ctx context.Context, obj *model.PrimussExam) ([]*model.StudentReg, error) {
	return r.plexams.GetStudentRegs(ctx, obj)
}

// Conflicts is the resolver for the conflicts field.
func (r *primussExamResolver) Conflicts(ctx context.Context, obj *model.PrimussExam) (*model.Conflicts, error) {
	return r.plexams.GetConflicts(ctx, obj)
}

// PrimussExam returns generated.PrimussExamResolver implementation.
func (r *Resolver) PrimussExam() generated.PrimussExamResolver { return &primussExamResolver{r} }

type primussExamResolver struct{ *Resolver }
