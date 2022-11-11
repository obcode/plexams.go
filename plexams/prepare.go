package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrepareStudentRegs() error {
	ctx := context.Background()
	studentRegsPerAncode, studentRegsPerStudent, err := p.prepareAllStudentRegs()
	log.Debug().Int("length studentRegsPerAncode", len(studentRegsPerAncode)).
		Int("length studentRegsPerStudent", len(studentRegsPerStudent)).Msg("got the following")
	if err != nil {
		return err
	}

	ancodes, err := p.dbClient.GetZpaAncodesPlanned(ctx)
	if err != nil {
		return err
	}

	studentRegsPerAncodePlanned := make(map[int]map[string][]*model.StudentReg)
	for _, ancode := range ancodes {
		studentRegsPerAncodePlanned[ancode.AnCode] = studentRegsPerAncode[ancode.AnCode]
	}

	studentRegsPerStudentPlanned := make(map[string][]*model.StudentReg, 0)
	for mtknr, studentRegs := range studentRegsPerStudent {
		studentRegsPlanned := make([]*model.StudentReg, 0)
		for _, studentReg := range studentRegs {
			for _, ancode := range ancodes {
				if ancode.AnCode == studentReg.AnCode {
					studentRegsPlanned = append(studentRegsPlanned, studentReg)
					break
				}
			}
		}
		studentRegsPerStudentPlanned[mtknr] = studentRegsPlanned
	}

	err = p.dbClient.SaveStudentRegsPerAncode(ctx, studentRegsPerAncode, true)
	if err != nil {
		return err
	}

	err = p.dbClient.SaveStudentRegsPerAncode(ctx, studentRegsPerAncodePlanned, false)
	if err != nil {
		return err
	}

	err = p.dbClient.SaveStudentRegsPerStudent(ctx, studentRegsPerStudent, true)
	if err != nil {
		return err
	}

	err = p.dbClient.SaveStudentRegsPerStudent(ctx, studentRegsPerStudentPlanned, false)
	if err != nil {
		return err
	}

	return nil
}

func (p *Plexams) prepareAllStudentRegs() (
	studentRegsPerAncode map[int]map[string][]*model.StudentReg,
	studentRegsPerStudent map[string][]*model.StudentReg,
	err error,
) {
	ctx := context.Background()
	studentRegsPerAncode = make(map[int]map[string][]*model.StudentReg)
	studentRegsPerStudent = make(map[string][]*model.StudentReg)

	programs, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return
	}

	for _, program := range programs {
		var studentRegsPerAncodeForProgram map[int][]*model.StudentReg
		studentRegsPerAncodeForProgram, err = p.dbClient.GetPrimussStudentRegsPerAncode(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get studentregs for program")
			return
		}

		for ancode, studentRegs := range studentRegsPerAncodeForProgram {
			// per ancodes
			regs, ok := studentRegsPerAncode[ancode]
			if !ok {
				regs = make(map[string][]*model.StudentReg)
			}
			regs[program] = studentRegs
			studentRegsPerAncode[ancode] = regs

			// per student
			for _, studentReg := range studentRegs {
				regs, ok := studentRegsPerStudent[studentReg.Mtknr]
				if !ok {
					regs = make([]*model.StudentReg, 0)
				}
				regs = append(regs, studentReg)
				studentRegsPerStudent[studentReg.Mtknr] = regs
			}
		}
	}

	return
}
