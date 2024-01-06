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

func (p *Plexams) ValidateDB() error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating data base entries")),
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

	validationMessages := make([]string, 0)

	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planEntries")
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow(" validating only one plan entry per ancode")))
	planEntryMap := make(map[int]*model.PlanEntry)
	for _, planEntry := range planEntries {
		otherEntry, ok := planEntryMap[planEntry.Ancode]
		if ok {
			validationMessages = append(validationMessages, aurora.Sprintf(
				aurora.Red("more than one plan entry for ancode %d: %v and %v"),
				aurora.Magenta(planEntry.Ancode),
				aurora.Cyan(otherEntry), aurora.Cyan(planEntry),
			))
		}
	}

	spinner.Message(aurora.Sprintf(aurora.Yellow(" validating correct start times")))
	for _, planEntry := range planEntries {
		slottime := p.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
		if !slottime.Equal(planEntry.Starttime) {
			validationMessages = append(validationMessages, aurora.Sprintf(
				aurora.Red("wrong starttime for ancode %d: want %s, got %s"),
				aurora.Magenta(planEntry.Ancode),
				aurora.Cyan(slottime.Local().Format("02.01.06, 15:04")),
				aurora.Cyan(planEntry.Starttime.Local().Format("02.01.06, 15:04")),
			))
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems"),
			len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("%d plan entries are okay"),
			len(planEntries)))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}
