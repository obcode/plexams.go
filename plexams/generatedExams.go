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

func (p *Plexams) PrepareGeneratedExams() error {
	ctx := context.Background()
	// from connected exams to exam generated
	connectedExams, err := p.GetConnectedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return err
	}

	// TODO: connectedExamsMap

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
		ntaMap[nta.Mtknr] = nta
	}

	externalExams, err := p.ExternalExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get external exams")
		return err
	}

	ancodesMap := primussAncodesToZpaAncodes(connectedExams, externalExams)

	exams := make([]*model.GeneratedExam, 0, len(connectedExams))

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
			enhanced, err := p.primussToEnhanced(ctx, primussExam, ntaMap)
			if err != nil {
				log.Error().Err(err).Str("program", primussExam.Program).Int("ancode", primussExam.AnCode).
					Msg("cannot enhance primuss exam")
				return err
			}

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

		duration := connectedExam.ZpaExam.Duration
		maxDuration := duration
		for _, nta := range ntas {
			ntaDuration := (duration * (100 + nta.DeltaDurationPercent)) / 100
			if ntaDuration > maxDuration {
				maxDuration = ntaDuration
			}
		}

		exams = append(exams, &model.GeneratedExam{
			Ancode:           connectedExam.ZpaExam.AnCode,
			ZpaExam:          connectedExam.ZpaExam,
			PrimussExams:     enhancedPrimussExams,
			Constraints:      constraints[connectedExam.ZpaExam.AnCode],
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

	return p.dbClient.CacheGeneratedExams(ctx, exams)
}

func (p *Plexams) primussToEnhanced(ctx context.Context, exam *model.PrimussExam, ntaMap map[string]*model.NTA) (*model.EnhancedPrimussExam, error) {
	studentRegs, err := p.GetStudentRegs(ctx, exam)
	if err != nil {
		log.Error().Err(err).Str("program", exam.Program).Int("ancode", exam.AnCode).
			Msg("cannot get student regs for primuss exam")
		return nil, err
	}

	conflicts, err := p.GetConflicts(ctx, exam)
	if err != nil {
		log.Error().Err(err).Str("program", exam.Program).Int("ancode", exam.AnCode).
			Msg("cannot get student regs for primuss exam")
		return nil, err
	}

	ntas := make([]*model.NTA, 0)

	for _, studentReg := range studentRegs {
		nta, ok := ntaMap[studentReg.Mtknr]
		if ok {
			ntas = append(ntas, nta)
		}
	}

	if len(studentRegs) > 0 && !p.dbClient.CheckStudentRegsCount(ctx, exam.Program, exam.AnCode, len(studentRegs)) {
		log.Error().Err(err).Str("program", exam.Program).Int("ancode", exam.AnCode).Int("count", len(studentRegs)).
			Msg("student reg count does not match")
	}

	return &model.EnhancedPrimussExam{
		Exam:        exam,
		StudentRegs: studentRegs,
		Conflicts:   conflicts.Conflicts,
		Ntas:        ntas,
	}, nil
}

type PrimussAncode struct {
	Program string
	Ancode  int
}

func primussAncodesToZpaAncodes(exams []*model.ConnectedExam, externalExams []*model.ExternalExam) map[PrimussAncode]int {
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

func (p *Plexams) GeneratedExams(ctx context.Context) ([]*model.GeneratedExam, error) {
	return p.dbClient.GetGeneratedExams(ctx)
}

// GeneratedExam is the resolver for the generatedExam field.
func (p *Plexams) GeneratedExam(ctx context.Context, ancode int) (*model.GeneratedExam, error) {
	return p.dbClient.GetGeneratedExam(ctx, ancode)
}

// ConflictingAncodes is the resolver for the conflictingAncodes field.
func (p *Plexams) ConflictingAncodes(ctx context.Context, ancode int) ([]*model.Conflict, error) {
	exam, err := p.dbClient.GetGeneratedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get generated exam")
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
