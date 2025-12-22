package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	return p.dbClient.GetPrimussExams(ctx)
}

func (p *Plexams) GetPrimussExam(ctx context.Context, program string, ancode int) (*model.PrimussExam, error) {
	return p.dbClient.GetPrimussExam(ctx, program, ancode)
}

func (p *Plexams) PrimussExamExists(ctx context.Context, program string, ancode int) (bool, error) {
	return p.dbClient.PrimussExamExists(ctx, program, ancode)
}

func (p *Plexams) GetPrimussExamsForAncode(ctx context.Context, ancode int) ([]*model.PrimussExam, error) {
	return p.dbClient.GetPrimussExamsForAncode(ctx, ancode)
}

func (p *Plexams) GetStudentRegs(ctx context.Context, exam *model.PrimussExam) ([]*model.StudentReg, error) {
	return p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, exam.Program, exam.AnCode)
}

func (p *Plexams) GetEnhancedStudentRegs(ctx context.Context, program string, ancode int) ([]*model.EnhancedStudentReg, error) {
	studentRegs, err := p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, program, ancode)
	if err != nil {
		return nil, err
	}

	enhancedStudentRegs := make([]*model.EnhancedStudentReg, 0, len(studentRegs))
	for _, studentReg := range studentRegs {
		zpaStudent, err := p.dbClient.GetZPAStudentByMtknr(ctx, studentReg.Mtknr)
		if err != nil {
			log.Debug().Err(err).Str("mtknr", studentReg.Mtknr).Msg("cannot find zpa student for student reg")
		}
		enhancedStudentRegs = append(enhancedStudentRegs, &model.EnhancedStudentReg{
			ZpaStudent: zpaStudent,
			Mtknr:      studentReg.Mtknr,
			Ancode:     studentReg.AnCode,
			Program:    studentReg.Program,
			Group:      studentReg.Group,
			Name:       studentReg.Name,
			Presence:   studentReg.Presence,
		})
	}
	return enhancedStudentRegs, nil
}

func (p *Plexams) StudentRegsForProgram(ctx context.Context, program string) ([]*model.StudentReg, error) {
	return p.dbClient.StudentRegsForProgram(ctx, program)
}

func (p *Plexams) StudentRegsPerStudentPlanned(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.StudentRegsPerStudentPlanned(ctx)
}

func (p *Plexams) StudentRegsImportErrors(ctx context.Context) ([]*model.RegWithError, error) {
	return p.dbClient.GetRegsWithErrors(ctx)
}

func (p *Plexams) GetConflicts(ctx context.Context, exam *model.PrimussExam) (*model.Conflicts, error) {
	return p.dbClient.GetPrimussConflictsForAncode(ctx, exam.Program, exam.AnCode)
}

func (p *Plexams) AddAncode(ctx context.Context, zpaAncode int, program string, primussAncode int) error {
	return p.dbClient.AddAncode(ctx, zpaAncode, program, primussAncode)
}
