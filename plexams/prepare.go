package plexams

import (
	"context"
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrepareStudentRegs() error {
	ctx := context.Background()

	programs, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return err
	}

	connectedExams, err := p.dbClient.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return err
	}

	type programmAndAncode struct {
		Program string `json:"program,omitempty"`
		Ancode  int    `json:"ancode,omitempty"`
	}

	plannedZpaAncodes := set.NewSet[int]()
	primussAncodesToZpaAncodes := make(map[programmAndAncode]int)

	for _, connectedExam := range connectedExams {
		ancode := connectedExam.ZpaExam.AnCode

		plannedZpaAncodes.Add(ancode)

		for _, primussExam := range connectedExam.PrimussExams {
			if primussExam.AnCode != ancode {
				primussAncodesToZpaAncodes[programmAndAncode{primussExam.Program, primussExam.AnCode}] = ancode
			}
		}
	}

	for k, v := range primussAncodesToZpaAncodes {
		log.Debug().Interface("primussAncode", k).Int("zpa ancode", v).Msg("primuss ancodes with different zpa ancodes")
	}

	// mtknr -> studentreg
	studentRegsPerStudent := make(map[string][]*model.StudentReg)

	for _, program := range programs {
		studentRegs, err := p.dbClient.StudentRegsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get studentregs for program")
			return err
		}

		for _, studentReg := range studentRegs {
			zpaAncode, ok := primussAncodesToZpaAncodes[programmAndAncode{program, studentReg.AnCode}]
			if ok {
				log.Debug().Str("program", program).Int("primussAncode", studentReg.AnCode).Int("zpaAncode", zpaAncode).
					Str("name", studentReg.Name).Msg("fixing ancode")
				studentReg.AnCode = zpaAncode
			}

			if !plannedZpaAncodes.Contains(studentReg.AnCode) {
				continue
			}

			otherRegs, ok := studentRegsPerStudent[studentReg.Mtknr]
			if ok {
				studentRegsPerStudent[studentReg.Mtknr] = append(otherRegs, studentReg)
			} else {
				studentRegsPerStudent[studentReg.Mtknr] = []*model.StudentReg{studentReg}
			}
		}
	}

	ntas, err := p.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	ntaMap := make(map[string]*model.NTA)
	for _, nta := range ntas {
		ntaMap[nta.Mtknr] = nta
	}

	studentRegsSlice := make([]interface{}, 0)

	for mtknr, regs := range studentRegsPerStudent {
		if len(regs) > 0 {
			ancodes := make([]int, 0, len(regs))
			for _, reg := range regs {
				ancodes = append(ancodes, reg.AnCode)
			}

			sort.Ints(ancodes)

			studentRegsSlice = append(studentRegsSlice, &model.Student{
				Mtknr:   mtknr,
				Program: regs[0].Program,
				Group:   regs[0].Group,
				Name:    regs[0].Name,
				Regs:    ancodes,
				Nta:     ntaMap[mtknr],
			})
		}
	}

	return p.dbClient.SaveStudentRegs(ctx, studentRegsSlice)
}

// Deprecated: part of generated exams
func (p *Plexams) PrepareExamsWithRegs() error {
	ctx := context.Background()
	zpaExamsToPlan, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	connectedExamsSlice, err := p.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
	}

	connectedExams := make(map[int]*model.ConnectedExam)
	for _, connectedExam := range connectedExamsSlice {
		connectedExams[connectedExam.ZpaExam.AnCode] = connectedExam
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
				Program:   studRegs.Program,
				Conflicts: conflictsProgAncode.Conflicts,
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
