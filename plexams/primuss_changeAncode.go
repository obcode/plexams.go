package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) ChangeAncode(ctx context.Context, program string, anCode, newAncode int) (*model.PrimussExam, error) {
	return p.dbClient.ChangeAncode(ctx, program, anCode, newAncode)
}

func (p *Plexams) ChangeAncodeInStudentRegs(ctx context.Context, program string, anCode, newAncode int) ([]*model.StudentReg, error) {
	return p.dbClient.ChangeAncodeInStudentRegs(ctx, program, anCode, newAncode)
}

func (p *Plexams) ChangeAncodeInConflicts(ctx context.Context, program string, anCode, newAncode int) (*model.Conflicts, error) {
	return p.dbClient.ChangeAncodeInConflicts(ctx, program, anCode, newAncode)
}
