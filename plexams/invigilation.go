package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GetInvigilatorInSlot(ctx context.Context, roomname string, day, time int) (*model.Teacher, error) {
	return p.dbClient.GetInvigilatorInSlot(ctx, roomname, day, time)
}

func (p *Plexams) AddInvigilation(ctx context.Context, room string, day, slot, invigilatorID int) error {
	invigilator, err := p.GetInvigilator(ctx, invigilatorID)
	if err != nil {
		return err
	}
	// check if (day,slot) needs a reserve, i.e. contains exams
	examsInSlot, err := p.ExamsInSlot(ctx, day, slot)
	if err != nil {
		return err
	}
	if len(examsInSlot) == 0 {
		return fmt.Errorf("need no invigilation in slot without exams")
	}

	// check constraints
	// excluded day
	for _, excludedDay := range invigilator.Requirements.ExcludedDays {
		if day == excludedDay {
			return fmt.Errorf("cannot add invigilation on excluded day for %s", invigilator.Teacher.Shortname)
		}
	}
	// no exam in same slot
	exams, err := p.dbClient.PlannedExamsByMainExamer(ctx, invigilatorID)
	if err != nil {
		return err
	}
	for _, exam := range exams {
		if exam.Slot.DayNumber == day && exam.Slot.SlotNumber == slot {
			return fmt.Errorf("cannot add invigilation, %s has own exam in slot", invigilator.Teacher.Shortname)
		}
	}
	// add to DB
	return p.dbClient.AddInvigilation(context.Background(), room, day, slot, invigilatorID)
}

func (p *Plexams) GetInvigilator(ctx context.Context, invigilatorID int) (*model.Invigilator, error) {
	invigilationTodos, err := p.InvigilationTodos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilation todos")
		return nil, err
	}
	// check if invigilatorID is invigilator
	var invigilator *model.Invigilator
	for _, knownInvigilator := range invigilationTodos.Invigilators {
		if knownInvigilator.Teacher.ID == invigilatorID {
			invigilator = knownInvigilator
			break
		}
	}

	if invigilator == nil {
		return nil, fmt.Errorf("invigilator with id %d not found", invigilatorID)
	}

	return invigilator, nil
}
