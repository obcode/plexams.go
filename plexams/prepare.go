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

	plannedZpaAncodes := make(map[string]set.Set[int]) // program -> set of ancodes
	primussAncodesToZpaAncodes := make(map[programmAndAncode]int)

	for _, connectedExam := range connectedExams {
		ancode := connectedExam.ZpaExam.AnCode

		for _, primussExam := range connectedExam.PrimussExams {
			plannedZpaAncodesForProgram, ok := plannedZpaAncodes[primussExam.Program]
			if !ok {
				plannedZpaAncodesForProgram = set.NewSet[int]()
			}
			plannedZpaAncodesForProgram.Add(ancode)
			plannedZpaAncodes[primussExam.Program] = plannedZpaAncodesForProgram

			if primussExam.AnCode != ancode {
				primussAncodesToZpaAncodes[programmAndAncode{primussExam.Program, primussExam.AnCode}] = ancode
			}
		}
	}

	for k, v := range primussAncodesToZpaAncodes {
		log.Debug().Interface("primussAncode", k).Int("zpa ancode", v).Msg("primuss ancodes with different zpa ancodes")
	}

	type studentRegFromProgram struct {
		program    string
		studentReg *model.StudentReg
	}

	// mtknr -> studentreg
	studentRegsPerStudent := make(map[string][]studentRegFromProgram)

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

			if !plannedZpaAncodes[program].Contains(studentReg.AnCode) {
				continue
			}

			otherRegs, ok := studentRegsPerStudent[studentReg.Mtknr]
			if ok {
				studentRegsPerStudent[studentReg.Mtknr] = append(otherRegs, studentRegFromProgram{program, studentReg})
			} else {
				studentRegsPerStudent[studentReg.Mtknr] = []studentRegFromProgram{{program, studentReg}}
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
		if !nta.Deactivated {
			log.Debug().Str("mtknr", nta.Mtknr).Msg("adding nta")
			ntaMap[nta.Mtknr] = nta
		} else {
			log.Debug().Str("mtknr", nta.Mtknr).Msg("NOT adding deactivated nta")
		}
	}

	studentRegsSlice := make([]interface{}, 0)

	for mtknr, regs := range studentRegsPerStudent {
		if len(regs) > 0 {
			ancodeSet := set.NewSet[int]()
			for _, reg := range regs {
				ancodeSet.Add(reg.studentReg.AnCode)
			}

			ancodes := ancodeSet.ToSlice()
			sort.Ints(ancodes)

			regsWithProgram := make([]*model.RegWithProgram, 0, len(regs))
			for _, reg := range regs {
				regsWithProgram = append(regsWithProgram, &model.RegWithProgram{
					Program: reg.program,
					Reg:     reg.studentReg.AnCode,
				})
			}

			zpaStudent, err := p.dbClient.GetZPAStudentByMtknr(ctx, mtknr)
			if err != nil {
				log.Debug().Err(err).Str("mtknr", mtknr).Msg("cannot get zpa student")
			}

			nta, ok := ntaMap[mtknr]
			if ok {
				if regs[0].studentReg.Program != nta.Program {
					log.Warn().Str("mtknr", mtknr).
						Str("name", regs[0].studentReg.Name).
						Str("studentRegProgram", regs[0].studentReg.Program).
						Str("ntaProgram", nta.Program).
						Msg("mismatch between studentreg program and nta program => not setting nta")
					nta = nil
				}
			}

			studentRegsSlice = append(studentRegsSlice, &model.Student{
				Mtknr:           mtknr,
				Program:         regs[0].studentReg.Program,
				Group:           regs[0].studentReg.Group,
				Name:            regs[0].studentReg.Name,
				Regs:            ancodes,
				RegsWithProgram: regsWithProgram,
				Nta:             nta,
				ZpaStudent:      zpaStudent,
			})
		}
	}

	err = p.dbClient.SetSemesterOnNTAs(ctx, studentRegsSlice)
	if err != nil {
		log.Error().Err(err).Msg("cannot set last semester on ntas")
	}

	return p.dbClient.SaveStudentRegs(ctx, studentRegsSlice)
}
