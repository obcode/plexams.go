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

func (p *Plexams) RemovePrimussExam(ctx context.Context, input model.PrimussExamInput) (bool, error) {
	// TODO: Implement me
	// wenn schon in DB, dann einzelne Prüfung herausnehmen und updaten
	// if true {
	// 	oks := true
	// 	for _, input := range input {
	// 		ok, err := p.RemovePrimussExam(ctx, *input)
	// 		oks = oks && ok
	// 		if err != nil {
	// 			log.Error().Err(err).
	// 				Int("anCode", input.AnCode).Str("program", input.Program).
	// 				Msg("cannot remove primuss exam")
	// 			return oks, err
	// 		}
	// 	}
	// 	return oks, nil
	// }
	return true, nil
}
