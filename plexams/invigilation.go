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
	for _, examInSlot := range examsInSlot {
		if examInSlot.ZpaExam.MainExamerID == invigilator.Teacher.ID {
			return fmt.Errorf("cannot add invigilation, %s has own exam in slot", invigilator.Teacher.Shortname)
		}
	}

	// add to DB
	return p.dbClient.AddInvigilation(context.Background(), room, day, slot, invigilatorID)
}

// PreAddInvigilation fixes an invigilator for a room (roomName != nil) or the
// reserve (roomName == nil) in a slot before the automatic invigilation
// planning runs. It validates the same constraints as AddInvigilation.
func (p *Plexams) PreAddInvigilation(ctx context.Context, invigilatorID, day, slot int, roomName *string) (bool, error) {
	invigilator, err := p.GetInvigilator(ctx, invigilatorID)
	if err != nil {
		return false, err
	}

	// check if (day,slot) needs an invigilation, i.e. contains exams
	examsInSlot, err := p.ExamsInSlot(ctx, day, slot)
	if err != nil {
		return false, err
	}
	if len(examsInSlot) == 0 {
		return false, fmt.Errorf("need no invigilation in slot without exams")
	}

	// check constraints
	// excluded day
	for _, excludedDay := range invigilator.Requirements.ExcludedDays {
		if day == excludedDay {
			return false, fmt.Errorf("cannot pre-plan invigilation on excluded day for %s", invigilator.Teacher.Shortname)
		}
	}
	// no own exam in same slot
	for _, examInSlot := range examsInSlot {
		if examInSlot.ZpaExam.MainExamerID == invigilator.Teacher.ID {
			return false, fmt.Errorf("cannot pre-plan invigilation, %s has own exam in slot", invigilator.Teacher.Shortname)
		}
	}

	// if a room is given, it must be planned in the slot
	isReserve := true
	if roomName != nil {
		isReserve = false
		roomnames, err := p.PlannedRoomNamesInSlot(ctx, day, slot)
		if err != nil {
			return false, err
		}
		found := false
		for _, name := range roomnames {
			if *roomName == name {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Errorf("room %s not found in slot (%d,%d)", *roomName, day, slot)
		}
	}

	return p.dbClient.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: invigilatorID,
		Day:           day,
		Slot:          slot,
		RoomName:      roomName,
		IsReserve:     isReserve,
	})
}

func (p *Plexams) PrePlannedInvigilations(ctx context.Context) ([]*model.PrePlannedInvigilation, error) {
	return p.dbClient.PrePlannedInvigilations(ctx)
}

// PrePlanInvigilationInSlot promotes the invigilation currently planned for a
// room (roomName != nil) or the reserve (roomName == nil) in a slot to a
// pre-planned, fixed assignment. It looks up the assigned invigilator, stores a
// pre-planned invigilation for them and marks the live invigilation as
// pre-planned so the GUI can show it.
func (p *Plexams) PrePlanInvigilationInSlot(ctx context.Context, day, slot int, roomName *string) (bool, error) {
	room := "reserve"
	if roomName != nil {
		room = *roomName
	}

	teacher, err := p.dbClient.GetInvigilatorInSlot(ctx, room, day, slot)
	if err != nil {
		return false, err
	}
	if teacher == nil {
		return false, fmt.Errorf("no invigilation for %s in slot (%d,%d) to pre-plan", room, day, slot)
	}

	ok, err := p.PreAddInvigilation(ctx, teacher.ID, day, slot, roomName)
	if err != nil || !ok {
		return false, err
	}

	if err := p.dbClient.SetInvigilationPrePlanned(ctx, day, slot, roomName, true); err != nil {
		return false, err
	}
	return true, nil
}

func (p *Plexams) PrePlannedInvigilationsForInvigilator(ctx context.Context, invigilatorID int) ([]*model.PrePlannedInvigilation, error) {
	return p.dbClient.PrePlannedInvigilationsForInvigilator(ctx, invigilatorID)
}

func (p *Plexams) GetInvigilator(ctx context.Context, invigilatorID int) (*model.Invigilator, error) {
	invigilationTodos, err := p.dbClient.GetInvigilationTodos(ctx)
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
