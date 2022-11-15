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
		studentRegsPerAncodePlanned[ancode.Ancode] = studentRegsPerAncode[ancode.Ancode]
	}

	studentRegsPerStudentPlanned := make(map[string][]*model.StudentReg, 0)
	for mtknr, studentRegs := range studentRegsPerStudent {
		studentRegsPlanned := make([]*model.StudentReg, 0)
		for _, studentReg := range studentRegs {
			for _, ancode := range ancodes {
				if ancode.Ancode == studentReg.AnCode {
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

func (p *Plexams) PrepareExamsWithRegs() error {
	ctx := context.Background()
	zpaExamsToPlan, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	// zpaExamsToPlan := make(map[int]*model.ZPAExam)
	// for _, exam := range zpaExamsSlice {
	// 	zpaExamsToPlan[exam.AnCode] = exam
	// }

	connectedExamsSlice, err := p.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
	}

	connectedExams := make(map[int]*model.ConnectedExam)
	for _, connecconnectedExam := range connectedExamsSlice {
		connectedExams[connecconnectedExam.ZpaExam.AnCode] = connecconnectedExam
	}

	studentRegsSlice, err := p.GetStudentRegsPerAncodePlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
	}

	studentRegs := make(map[int]*model.StudentRegsPerAncode)
	for _, studentReg := range studentRegsSlice {
		studentRegs[studentReg.Ancode] = studentReg
	}

	// combine the exams with regs

	examsWithRegs := make([]*model.ExamWithRegs, 0, len(zpaExamsToPlan))
	for _, zpaExam := range zpaExamsToPlan {
		ancode := zpaExam.AnCode
		primussExams := connectedExams[ancode].PrimussExams
		studentRegsForAncode := studentRegs[ancode].PerProgram

		studentRegsForExam := make([]*model.StudentRegsPerAncodeAndProgram, 0, len(primussExams))
		for _, primussExam := range primussExams {
			for _, studRegs := range studentRegsForAncode {
				if primussExam.Program == studRegs.Program {
					studentRegsForExam = append(studentRegsForExam, studRegs)
				}
			}
		}

		conflicts := make([]*model.ConflictPerProgram, 0)
		for _, studRegs := range studentRegsForExam {
			conflictsProgAncode, err := p.dbClient.GetPrimussConflictsForAncodeOnlyPlanned(ctx, studRegs.Program, ancode, zpaExamsToPlan)
			if err != nil {
				log.Error().Err(err).Str("program", studRegs.Program).Int("ancode", ancode).
					Msg("cannot get conflicts")
			}
			conflicts = append(conflicts, &model.ConflictPerProgram{
				Program:  studRegs.Program,
				Conflics: conflictsProgAncode.Conflicts,
			})
		}

		examWithReg := model.ExamWithRegs{
			Ancode:        ancode,
			ZpaExam:       zpaExam,
			PrimussExams:  primussExams,
			StudentRegs:   studentRegsForExam,
			Conflicts:     conflicts,
			ConnectErrors: connectedExams[ancode].Errors,
		}
		examsWithRegs = append(examsWithRegs, &examWithReg)
	}

	return p.dbClient.SaveExamsWithRegs(ctx, examsWithRegs)
}
