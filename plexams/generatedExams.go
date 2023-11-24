package plexams

import (
	"context"
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

	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get connected exams")
		return err
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

	externalExams, err := p.ExternalExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get external exams")
		return err
	}

	ancodesMap := primussAncodesToZpaAncodes(connectedExams, externalExams)

	exams := make([]*model.GeneratedExam, 0, len(connectedExams))

	for _, connectedExam := range connectedExams {
		// // TODO: remove me
		// if connectedExam.ZpaExam.AnCode != 390 && connectedExam.ZpaExam.AnCode != 393 && connectedExam.ZpaExam.AnCode != 440 {
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

		spinner.Message("getting primuss exams")
		studentRegsCount := 0
		enhancedPrimussExams := make([]*model.EnhancedPrimussExam, 0, len(connectedExam.PrimussExams))
		for _, primussExam := range connectedExam.PrimussExams {
			enhanced, err := p.primussToEnhanced(ctx, primussExam, ntaMap)
			if err != nil {
				log.Error().Err(err).Str("program", primussExam.Program).Int("ancode", primussExam.AnCode).
					Msg("cannot enhance primuss exam")
				return err
			}

			studentRegsCount += len(enhanced.StudentRegs)
			enhancedPrimussExams = append(enhancedPrimussExams, enhanced)
		}

		spinner.Message("recalculating conflicts")

		conflictsMap := make(map[int]*model.ZPAConflict)
		for _, enhanced := range enhancedPrimussExams {
			for _, primussConflict := range enhanced.Conflicts {
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

		exams = append(exams, &model.GeneratedExam{
			Ancode:           connectedExam.ZpaExam.AnCode,
			ZpaExam:          connectedExam.ZpaExam,
			PrimussExams:     enhancedPrimussExams,
			Constraints:      constraints[connectedExam.ZpaExam.AnCode],
			Conflicts:        conflicts,
			StudentRegsCount: studentRegsCount,
		})

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	// TODO: External Exams with student regs, ...
	// automatically set notPlannedByMe constraint

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
