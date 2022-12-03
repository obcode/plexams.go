package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) ChangeAncode(ctx context.Context, program string, ancode, newAncode int) (*model.PrimussExam, error) {
	return p.dbClient.ChangeAncode(ctx, program, ancode, newAncode)
}

func (p *Plexams) ChangeAncodeInStudentRegs(ctx context.Context, program string, ancode, newAncode int) ([]*model.StudentReg, error) {
	return p.dbClient.ChangeAncodeInStudentRegs(ctx, program, ancode, newAncode)
}

func (p *Plexams) ChangeAncodeInConflicts(ctx context.Context, program string, ancode, newAncode int) (*model.Conflicts, error) {
	return p.dbClient.ChangeAncodeInConflicts(ctx, program, ancode, newAncode)
}

func (p *Plexams) RemoveStudentReg(ctx context.Context, program string, ancode int, mtknr string) (int, error) {
	return p.dbClient.RemoveStudentReg(ctx, program, ancode, mtknr)
}
