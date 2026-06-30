package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) PrepareAssembledExams() error {
	ctx := context.Background()
	// from connected exams to exam generated
	connectedExams, err := p.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return err
	}

	// TODO: connectedExamsMap?

	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return err
	}

	allNtas, err := p.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	ntaMap := make(map[string]*model.NTA)
	for _, nta := range allNtas {
		if !nta.Deactivated {
			log.Debug().Str("mtknr", nta.Mtknr).Msg("adding nta")
			ntaMap[nta.Mtknr] = nta
		} else {
			log.Debug().Str("mtknr", nta.Mtknr).Msg("NOT adding deactivated nta")
		}
	}

	zpaStudents, err := p.GetZPAStudents(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("cannot get zpa students")
	}

	zpaStudentsMap := make(map[string]*model.ZPAStudent)
	for _, zpaStudent := range zpaStudents {
		zpaStudentsMap[zpaStudent.Mtknr] = zpaStudent
	}

	// per-ancode duration overrides (DB); applied only to exams with ZPA duration 0
	durationMap, err := p.examDurationOverridesMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam duration overrides")
		return err
	}

	// externalExams, err := p.ExternalExams(ctx)
	// if err != nil {
	// 	log.Error().Err(err).Msg("cannot get external exams")
	// 	return err
	// }

	// Batch-load student regs and conflicts once per program (indexed by ancode),
	// instead of one DB lookup per primuss exam.
	programsSet := make(map[string]bool)
	for _, connectedExam := range connectedExams {
		for _, primussExam := range connectedExam.PrimussExams {
			programsSet[primussExam.Program] = true
		}
	}
	studentRegsIdx := make(map[string]map[int][]*model.StudentReg, len(programsSet))
	conflictsIdx := make(map[string]map[int]*model.Conflicts, len(programsSet))
	for program := range programsSet {
		studentRegs, err := p.dbClient.GetPrimussStudentRegsPerAncode(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get student regs for program")
			return err
		}
		studentRegsIdx[program] = studentRegs

		conflicts, err := p.dbClient.GetPrimussConflictsPerAncode(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get conflicts for program")
			return err
		}
		conflictsIdx[program] = conflicts
	}

	// ancodesMap := primussAncodesToZpaAncodes(connectedExams, externalExams)
	ancodesMap := primussAncodesToZpaAncodes(connectedExams)

	exams := make([]*model.AssembledExam, 0, len(connectedExams))

	for _, connectedExam := range connectedExams {
		// // TODO: remove me
		// if connectedExam.ZpaExam.AnCode != 390 && connectedExam.ZpaExam.AnCode != 393 && connectedExam.ZpaExam.AnCode != 294 {
		// 	continue
		// }

		cfg := yacspin.Config{
			Frequency: 100 * time.Millisecond,
			CharSet:   yacspin.CharSets[69],
			Suffix: aurora.Sprintf(aurora.Cyan(" generating exam %d. %s (%s)"),
				aurora.Yellow(connectedExam.ZpaExam.AnCode),
				aurora.Magenta(connectedExam.ZpaExam.Module),
				aurora.Magenta(connectedExam.ZpaExam.MainExamer),
			),
			SuffixAutoColon:   true,
			StopCharacter:     "✓",
			StopColors:        []string{"fgGreen"},
			StopFailMessage:   "error",
			StopFailCharacter: "✗",
			StopFailColors:    []string{"fgRed"},
		}

		spinner, err := yacspin.New(cfg)
		if err != nil {
			log.Debug().Err(err).Msg("cannot create spinner")
		}
		err = spinner.Start()
		if err != nil {
			log.Debug().Err(err).Msg("cannot start spinner")
		}

		spinner.Message("adding primuss data")
		studentRegsCount := 0
		ntas := make([]*model.NTA, 0)

		enhancedPrimussExams := make([]*model.EnhancedPrimussExam, 0, len(connectedExam.PrimussExams))
		for _, primussExam := range connectedExam.PrimussExams {
			enhanced := enhancePrimussExam(primussExam, ntaMap, zpaStudentsMap, studentRegsIdx, conflictsIdx)

			ntas = append(ntas, enhanced.Ntas...)
			studentRegsCount += len(enhanced.StudentRegs)
			enhancedPrimussExams = append(enhancedPrimussExams, enhanced)
		}

		conflictsMap := make(map[int]*model.ZPAConflict)
		for _, enhanced := range enhancedPrimussExams {
			for _, primussConflict := range enhanced.Conflicts {
				if primussConflict.AnCode == enhanced.Exam.AnCode {
					continue
				}
				zpaAncode, ok := ancodesMap[PrimussAncode{
					Program: enhanced.Exam.Program,
					Ancode:  primussConflict.AnCode,
				}]
				if ok {
					zpaConflict, ok := conflictsMap[zpaAncode]
					if ok {
						conflictsMap[zpaAncode] = &model.ZPAConflict{
							Ancode:        zpaAncode,
							NumberOfStuds: zpaConflict.NumberOfStuds + primussConflict.NumberOfStuds,
							PrimussAncodes: append(zpaConflict.PrimussAncodes, &model.PrimussExamAncode{
								Ancode:        primussConflict.AnCode,
								Program:       enhanced.Exam.Program,
								NumberOfStuds: primussConflict.NumberOfStuds,
							}),
						}
					} else {
						conflictsMap[zpaAncode] = &model.ZPAConflict{
							Ancode:        zpaAncode,
							NumberOfStuds: primussConflict.NumberOfStuds,
							PrimussAncodes: []*model.PrimussExamAncode{{
								Ancode:        primussConflict.AnCode,
								Program:       enhanced.Exam.Program,
								NumberOfStuds: primussConflict.NumberOfStuds,
							}},
						}
					}
				}
			}
		}

		keys := make([]int, 0, len(conflictsMap))
		for key := range conflictsMap {
			keys = append(keys, key)
		}
		sort.Ints(keys)

		conflicts := make([]*model.ZPAConflict, 0, len(conflictsMap))
		for _, key := range keys {
			conflicts = append(conflicts, conflictsMap[key])
		}

		// duration override only for exams without a ZPA duration (0)
		if connectedExam.ZpaExam.Duration == 0 {
			if d, ok := durationMap[connectedExam.ZpaExam.AnCode]; ok {
				connectedExam.ZpaExam.Duration = d
			}
		}

		duration := connectedExam.ZpaExam.Duration
		maxDuration := duration
		for _, nta := range ntas {
			ntaDuration := (duration * (100 + nta.DeltaDurationPercent)) / 100
			if ntaDuration > maxDuration {
				maxDuration = ntaDuration
			}
		}

		constraints := constraints[connectedExam.ZpaExam.AnCode]
		if connectedExam.ZpaExam.AnCode >= 1000 {
			constraints = &model.Constraints{
				NotPlannedByMe: true,
			}
		}

		exams = append(exams, &model.AssembledExam{
			Ancode:           connectedExam.ZpaExam.AnCode,
			ZpaExam:          connectedExam.ZpaExam,
			PrimussExams:     enhancedPrimussExams,
			Constraints:      constraints,
			Conflicts:        conflicts,
			StudentRegsCount: studentRegsCount,
			Ntas:             ntas,
			MaxDuration:      maxDuration,
		})

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	// TODO: External Exams with student regs, ...
	// automatically set notPlannedByMe constraint

	// spinner.Message("recalculating conflicts")
	// TODO: for recalculating the conflicts wee need also the exams which have to be in the same slot
	// primussExamsForConflicts := make([]*model.EnhancedPrimussExam)

	conflictsPerAncode := make(map[int][]*model.ZPAConflict)
	for _, exam := range exams {
		conflictsPerAncode[exam.Ancode] = exam.Conflicts
	}

	fmt.Println()

	for _, exam := range exams {
		// // TODO: remove me
		// if exam.ZpaExam.AnCode != 390 && exam.ZpaExam.AnCode != 393 && exam.ZpaExam.AnCode != 294 {
		// 	continue
		// }

		if exam.Constraints != nil && len(exam.Constraints.SameSlot) > 0 {
			cfg := yacspin.Config{
				Frequency: 100 * time.Millisecond,
				CharSet:   yacspin.CharSets[69],
				Suffix: aurora.Sprintf(aurora.Cyan(" recalculating conflicts for exam %d. %s (%s) with same slot constraints %v, was %2d"),
					aurora.Yellow(exam.ZpaExam.AnCode),
					aurora.Magenta(exam.ZpaExam.Module),
					aurora.Magenta(exam.ZpaExam.MainExamer),
					aurora.Red(exam.Constraints.SameSlot),
					aurora.Red(len(exam.Conflicts)),
				),
				SuffixAutoColon:   true,
				StopCharacter:     "✓",
				StopColors:        []string{"fgGreen"},
				StopFailMessage:   "error",
				StopFailCharacter: "✗",
				StopFailColors:    []string{"fgRed"},
			}
			spinner, err := yacspin.New(cfg)
			if err != nil {
				log.Debug().Err(err).Msg("cannot create spinner")
			}
			err = spinner.Start()
			if err != nil {
				log.Debug().Err(err).Msg("cannot start spinner")
			}

			conflictsMap := make(map[int]*model.ZPAConflict)
			for _, conflict := range exam.Conflicts {
				conflictsMap[conflict.Ancode] = conflict
			}

			for _, ancode := range exam.Constraints.SameSlot {
				otherConflicts, ok := conflictsPerAncode[ancode]
				if !ok {
					spinner.Message("cannot get other conflicts")
				}

				for _, otherConflict := range otherConflicts {
					conflictEntry, ok := conflictsMap[otherConflict.Ancode]
					if !ok {
						conflictsMap[otherConflict.Ancode] = otherConflict
					} else {
						conflictsMap[otherConflict.Ancode] = &model.ZPAConflict{
							Ancode:         conflictEntry.Ancode,
							NumberOfStuds:  conflictEntry.NumberOfStuds + otherConflict.NumberOfStuds,
							PrimussAncodes: append(conflictEntry.PrimussAncodes, otherConflict.PrimussAncodes...),
						}
					}
				}
			}

			keys := make([]int, 0, len(conflictsMap))
			for key := range conflictsMap {
				keys = append(keys, key)
			}
			sort.Ints(keys)

			conflicts := make([]*model.ZPAConflict, 0, len(conflictsMap))
			for _, key := range keys {
				conflicts = append(conflicts, conflictsMap[key])
			}

			exam.Conflicts = conflicts

			spinner.StopMessage(aurora.Sprintf(aurora.Green("now %d constraints"),
				aurora.Red(len(exam.Conflicts)),
			))
			err = spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
		}
	}

	if err := p.dbClient.CacheAssembledExams(ctx, exams); err != nil {
		return err
	}
	// the cache is now in sync with its inputs again
	if err := p.dbClient.SetAssembledExamsDirty(ctx, false, "", time.Now()); err != nil {
		log.Error().Err(err).Msg("cannot clear generated-exams dirty flag")
	}
	p.markCondition(ctx, condAssembledExams)
	return nil
}

// enhancePrimussExam enriches a primuss exam with its student regs, conflicts and
// NTAs from the preloaded per-program indices (pure, no DB access).
func enhancePrimussExam(exam *model.PrimussExam, ntaMap map[string]*model.NTA, zpaStudents map[string]*model.ZPAStudent,
	studentRegsIdx map[string]map[int][]*model.StudentReg, conflictsIdx map[string]map[int]*model.Conflicts) *model.EnhancedPrimussExam {
	studentRegs := studentRegsIdx[exam.Program][exam.AnCode]

	var conflicts []*model.Conflict
	if c, ok := conflictsIdx[exam.Program][exam.AnCode]; ok && c != nil {
		conflicts = c.Conflicts
	}

	ntas := make([]*model.NTA, 0)
	enhancedStudentRegs := make([]*model.EnhancedStudentReg, 0, len(studentRegs))

	for _, studentReg := range studentRegs {
		if nta, ok := ntaMap[studentReg.Mtknr]; ok {
			ntas = append(ntas, nta)
		}
		enhancedStudentRegs = append(enhancedStudentRegs, &model.EnhancedStudentReg{
			Mtknr:      studentReg.Mtknr,
			Ancode:     studentReg.AnCode,
			Program:    studentReg.Program,
			Group:      studentReg.Group,
			Name:       studentReg.Name,
			Presence:   studentReg.Presence,
			ZpaStudent: zpaStudents[studentReg.Mtknr],
		})
	}

	return &model.EnhancedPrimussExam{
		Exam:        exam,
		StudentRegs: enhancedStudentRegs,
		Conflicts:   conflicts,
		Ntas:        ntas,
	}
}

type PrimussAncode struct {
	Program string
	Ancode  int
}

// func primussAncodesToZpaAncodes(exams []*model.ConnectedExam, externalExams []*model.ExternalExam) map[PrimussAncode]int {
func primussAncodesToZpaAncodes(exams []*model.ConnectedExam) map[PrimussAncode]int {
	ancodesMap := make(map[PrimussAncode]int)
	for _, exam := range exams {
		zpaAncode := exam.ZpaExam.AnCode
		for _, primussExam := range exam.PrimussExams {
			ancodesMap[PrimussAncode{
				Program: primussExam.Program,
				Ancode:  primussExam.AnCode,
			}] = zpaAncode
		}
	}

	// TODO: add external exams

	return ancodesMap
}

func (p *Plexams) AssembledExams(ctx context.Context) ([]*model.AssembledExam, error) {
	return p.dbClient.GetAssembledExams(ctx)
}

func (p *Plexams) AssembledExamsForExamer(ctx context.Context, examerID int) ([]*model.AssembledExam, error) {
	return p.dbClient.GetAssembledExamsForExamer(ctx, examerID)
}

func (p *Plexams) AssembledExam(ctx context.Context, ancode int) (*model.AssembledExam, error) {
	return p.dbClient.GetAssembledExam(ctx, ancode)
}

func (p *Plexams) ConflictingAncodes(ctx context.Context, ancode int) ([]*model.Conflict, error) {
	exam, err := p.dbClient.GetAssembledExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, err
	}

	conflicts := make([]*model.Conflict, 0, len(exam.Conflicts))
	for _, zpaConflict := range exam.Conflicts {
		conflicts = append(conflicts, &model.Conflict{
			AnCode:        zpaConflict.Ancode,
			NumberOfStuds: zpaConflict.NumberOfStuds,
		})
	}

	return conflicts, nil
}
