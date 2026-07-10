package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GetStudentRegsForAncode(ancode int) (*model.StudentRegsForAncode, error) {
	ctx := context.TODO()
	f := false
	zpaExams, err := p.GetZPAExams(ctx, &f)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams")
	}

	var zpaExam *model.ZPAExam
	for _, exam := range zpaExams {
		if exam.AnCode == ancode {
			zpaExam = exam
			break
		}
	}
	if zpaExam == nil {
		log.Error().Int("ancode", ancode).Msg("ZPA exam not found")
		return nil, fmt.Errorf("exam %d not found", ancode)
	}
	log.Debug().Interface("exam", zpaExam).Msg("found ZPA exam")

	// The passed ancode is the internal ZPA ancode. Student registrations are stored per
	// program under the PRIMUSS ancode, which differs from the ZPA ancode for MUC.DAI /
	// external exams. So resolve the Primuss ancode per program from the exam's
	// PrimussAncodes (iterating those, not Groups, so external programs are hit too).
	studentRegs := make([]*model.StudentRegsPerAncodeAndProgram, 0, len(zpaExam.PrimussAncodes))
	for _, pa := range zpaExam.PrimussAncodes {
		if pa.Ancode <= 0 {
			continue // placeholder for a missing Primuss number — no regs to look up
		}
		program, primussAncode := pa.Program, pa.Ancode
		log.Debug().Str("program", program).Int("primussAncode", primussAncode).Int("zpaAncode", ancode).
			Msg("getting student regs for program")

		studentRegsForProgram, err := p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, program, primussAncode)
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("primussAncode", primussAncode).Msg("cannot get studentregs")
			return nil, err
		}
		studentRegs = append(studentRegs, &model.StudentRegsPerAncodeAndProgram{
			Program:       program,
			ZpaAncode:     ancode,
			PrimussAncode: primussAncode,
			StudentRegs:   studentRegsForProgram,
		})
	}

	return &model.StudentRegsForAncode{
		Exam:        zpaExam,
		StudentRegs: studentRegs,
	}, nil
}

func (p *Plexams) StudentByMtknr(ctx context.Context, mtknr string) (*model.Student, error) {
	return p.dbClient.StudentByMtknr(ctx, mtknr)
}

func (p *Plexams) StudentsByName(ctx context.Context, regex string) ([]*model.Student, error) {
	return p.dbClient.StudentsByName(ctx, regex)
}

func (p *Plexams) Students(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.StudentRegsPerStudentPlanned(ctx)
}
