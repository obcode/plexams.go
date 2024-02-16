package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) CacheExam(ancode int) error {
	cfg := yacspin.Config{
		Frequency: 100 * time.Millisecond,
		CharSet:   yacspin.CharSets[69],
		Suffix: aurora.Sprintf(aurora.Cyan(" caching exam %d"),
			aurora.Yellow(ancode),
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

	ctx := context.Background()

	spinner.Message("generating exam")
	exam, err := p.Exam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting exam")
		spinner.StopFailMessage(fmt.Sprintf("problem: %v", err))

		err := spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		return err
	}
	if exam.ZpaExam != nil {
		log.Debug().Int("ancode", exam.Ancode).Str("module", exam.ZpaExam.Module).Str("examer", exam.ZpaExam.MainExamer).
			Msg("caching exam")
	}

	spinner.Message("caching exam")
	err = p.dbClient.CacheExam(ctx, exam)
	if err == nil {
		if exam.ZpaExam != nil {
			str := aurora.Sprintf(aurora.Green("%s, %s"), exam.ZpaExam.MainExamer, exam.ZpaExam.Module)
			if len(exam.RegularStudents) > 0 {
				str = fmt.Sprintf("%s, %s", str, aurora.Sprintf(aurora.Magenta("%d stud"),
					len(exam.RegularStudents)+len(exam.NtaStudents)))
			}
			if len(exam.NtaStudents) > 0 {
				str = fmt.Sprintf("%s + %s", str, aurora.Sprintf(aurora.Red("%d nta"),
					len(exam.NtaStudents)))
			}

			spinner.StopMessage(str)
		}
	} else {
		spinner.StopFailMessage(fmt.Sprintf("problem: %v", err))

		err := spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}
	err = spinner.Stop()
	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}
	return nil
}

func (p *Plexams) CacheExams() error {
	ctx := context.Background()
	ancodes, err := p.GetZpaAnCodesToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa ancodes to plan")
		return err
	}
	for _, ancode := range ancodes {
		err = p.CacheExam(ancode.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode.Ancode).Msg("cannot cache exam")
			return err
		}
		log.Debug().Int("ancode", ancode.Ancode).Msg("successfully cached exam")
	}
	fmt.Println(aurora.Sprintf(aurora.Green("%d exams successfully cached.\n"), len(ancodes)))
	return nil
}

func (p *Plexams) RmCacheExams() error {
	return p.dbClient.RmCacheExams(context.Background())
}

func (p *Plexams) CachedExam(ctx context.Context, ancode int) (*model.Exam, error) {
	return p.dbClient.CachedExam(ctx, ancode)
}

func (p *Plexams) CachedExams(ctx context.Context) ([]*model.Exam, error) {
	return p.dbClient.CachedExams(ctx)
}
