package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrintSameName() {
	exams, err := p.GetZpaExamsToPlan(context.TODO())
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	sameNames := make(map[string][]*model.ZPAExam)

	for _, exam := range exams {
		_, ok := sameNames[exam.Module]
		if ok {
			sameNames[exam.Module] = append(sameNames[exam.Module], exam)
		} else {
			sameNames[exam.Module] = []*model.ZPAExam{exam}
		}
	}

	keys := make([]string, 0, len(sameNames))
	for k := range sameNames {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, name := range keys {
		sameNameExams := sameNames[name]
		if len(sameNameExams) > 1 {
			fmt.Printf("- %s\n", aurora.Blue(name))
			for _, e := range sameNameExams {
				fmt.Printf("  - %d. %s\n", aurora.Red(e.AnCode), aurora.Green(e.MainExamer))
			}
		}
	}
}
