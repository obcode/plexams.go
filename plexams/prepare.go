package plexams

import (
	"context"
	"sort"
	"time"

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

	connectedExams, err := p.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return err
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

	// A student registration carries both ancodes explicitly: primussAncode (external,
	// what the student registered on) and zpaAncode (internal, resolved via the connected
	// exams). The Primuss ancode is NO LONGER overwritten — both are kept side by side.
	type studentRegFromProgram struct {
		program       string
		primussAncode int
		zpaAncode     int
		studentReg    *model.StudentReg
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
			primussAncode, zpaAncode := resolveAncodes(program, studentReg.PrimussAncode, primussAncodesToZpaAncodes)

			if !plannedZpaAncodes[program].Contains(zpaAncode) {
				continue
			}

			reg := studentRegFromProgram{program, primussAncode, zpaAncode, studentReg}
			studentRegsPerStudent[studentReg.Mtknr] = append(studentRegsPerStudent[studentReg.Mtknr], reg)
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
			// zpaAncodes is the internal (deduplicated) list — the conflict/plan key. Two
			// Primuss ancodes of different programs may map to the same ZPA ancode (a MUC.DAI
			// aggregate exam): the set collapses them to one internal exam here, while
			// regsWithProgram below keeps both per-program entries.
			ancodeSet := set.NewSet[int]()
			for _, reg := range regs {
				ancodeSet.Add(reg.zpaAncode)
			}

			zpaAncodes := ancodeSet.ToSlice()
			sort.Ints(zpaAncodes)

			regsWithProgram := make([]*model.RegWithProgram, 0, len(regs))
			for _, reg := range regs {
				regsWithProgram = append(regsWithProgram, &model.RegWithProgram{
					Program:       reg.program,
					PrimussAncode: reg.primussAncode,
					ZpaAncode:     reg.zpaAncode,
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
				ZpaAncodes:      zpaAncodes,
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

	if err := p.dbClient.SaveStudentRegs(ctx, studentRegsSlice); err != nil {
		return err
	}
	// the prepared regs are now in sync with their inputs again
	if err := p.dbClient.SetStudentRegsDirty(ctx, false, "", time.Now()); err != nil {
		log.Error().Err(err).Msg("cannot clear student-regs dirty flag")
	}
	p.markCondition(ctx, condStudentRegs)
	return nil
}
