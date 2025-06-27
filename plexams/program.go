package plexams

import (
	"context"
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AllProgramsInPlan(ctx context.Context) ([]string, error) {
	exams, err := p.GeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams")
	}

	programSet := set.NewSet[string]()

	for _, exam := range exams {
		for _, primussExam := range exam.PrimussExams {
			programSet.Add(primussExam.Exam.Program)
		}
	}

	allPrograms := programSet.ToSlice()
	sort.Strings(allPrograms)

	return allPrograms, nil
}
