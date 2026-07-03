package plexams

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set/v2"
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

	groups := set.NewSet[string]()
	for _, group := range zpaExam.Groups {
		groups.Add(group[:2])
	}

	log.Debug().Interface("groups", groups).Msg("found the following groups")

	studentRegs := make([]*model.StudentRegsPerAncodeAndProgram, 0, groups.Cardinality())
	for _, program := range groups.ToSlice() {
		log.Debug().Str("program", program).Msg("getting student regs for program")

		studentRegsForProgram, err := p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, program, ancode)
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get studentregs")
			return nil, err
		}
		log.Debug().Str("program", program).Int("ancode", ancode).Interface("regs", studentRegsForProgram).Msg("found studentregs")
		studentRegs = append(studentRegs, &model.StudentRegsPerAncodeAndProgram{
			Program:     program,
			StudentRegs: studentRegsForProgram,
			Ancode:      ancode,
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
