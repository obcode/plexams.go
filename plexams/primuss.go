package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) PrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	return p.dbClient.GetPrimussExams(ctx)
}

func (p *Plexams) GetPrimussExam(ctx context.Context, program string, anCode int) (*model.PrimussExam, error) {
	return p.dbClient.GetPrimussExam(ctx, program, anCode)
}

func (p *Plexams) GetPrimussExamsForAncode(ctx context.Context, anCode int) ([]*model.PrimussExam, error) {
	return p.dbClient.GetPrimussExamsForAncode(ctx, anCode)
}

func (p *Plexams) GetStudentRegs(ctx context.Context, exam *model.PrimussExam) ([]*model.StudentReg, error) {
	return p.dbClient.GetPrimussStudentRegsForAncode(ctx, exam.Program, exam.AnCode)
}

func (p *Plexams) GetConflicts(ctx context.Context, exam *model.PrimussExam) (*model.Conflicts, error) {
	return p.dbClient.GetPrimussConflictsForAncode(ctx, exam.Program, exam.AnCode)
}
