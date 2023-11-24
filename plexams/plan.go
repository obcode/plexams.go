package plexams

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ExamsWithoutSlot(ctx context.Context) ([]*model.GeneratedExam, error) {
	exams, err := p.dbClient.GetGeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
		return nil, err
	}

	planEntries, err := p.dbClient.PlannedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned ancodes")
		return nil, err
	}

	examsWithotSlots := make([]*model.GeneratedExam, 0)

OUTER:
	for _, exam := range exams {
		for _, planEntry := range planEntries {
			if exam.Ancode == planEntry.Ancode {
				continue OUTER
			}
		}
		examsWithotSlots = append(examsWithotSlots, exam)
	}

	// sort by student regs
	examsMap := make(map[int][]*model.GeneratedExam)
	for _, exam := range examsWithotSlots {
		exams, ok := examsMap[exam.StudentRegsCount]
		if !ok {
			exams = make([]*model.GeneratedExam, 0, 1)
		}
		examsMap[exam.StudentRegsCount] = append(exams, exam)
	}

	keys := make([]int, 0, len(examsMap))
	for key := range examsMap {
		keys = append(keys, key)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(keys)))

	examsWithotSlotsSorted := make([]*model.GeneratedExam, 0, len(examsWithotSlots))
	for _, key := range keys {
		examsWithotSlotsSorted = append(examsWithotSlotsSorted, examsMap[key]...)
	}

	return examsWithotSlotsSorted, nil
}
