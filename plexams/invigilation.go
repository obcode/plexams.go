package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// GetInvigilatorAt returns the invigilator assigned to a room (or "reserve") at the
// given absolute start time.
func (p *Plexams) GetInvigilatorAt(ctx context.Context, roomname string, starttime time.Time) (*model.Teacher, error) {
	return p.dbClient.GetInvigilatorAt(ctx, roomname, starttime)
}

func (p *Plexams) AddInvigilation(ctx context.Context, room string, starttime time.Time, invigilatorID int) error {
	invigilator, err := p.GetInvigilator(ctx, invigilatorID)
	if err != nil {
		return err
	}
	// check if the slot needs a reserve, i.e. contains exams
	examsInSlot, err := p.ExamsAt(ctx, starttime)
	if err != nil {
		return err
	}
	if len(examsInSlot) == 0 {
		return fmt.Errorf("need no invigilation in slot without exams")
	}

	// check constraints
	// excluded day
	day := p.dayNumberForDate(starttime)
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
	return p.dbClient.AddInvigilationAt(context.Background(), room, starttime, invigilatorID)
}

// PreAddInvigilation fixes an invigilator for a room (roomName != nil) or the
// reserve (roomName == nil) at a start time before the automatic invigilation
// planning runs. It validates the same constraints as AddInvigilation.
func (p *Plexams) PreAddInvigilation(ctx context.Context, invigilatorID int, starttime time.Time, roomName *string) (bool, error) {
	invigilator, err := p.GetInvigilator(ctx, invigilatorID)
	if err != nil {
		return false, err
	}

	// check if the slot needs an invigilation, i.e. contains exams
	examsInSlot, err := p.ExamsAt(ctx, starttime)
	if err != nil {
		return false, err
	}
	if len(examsInSlot) == 0 {
		return false, fmt.Errorf("need no invigilation in slot without exams")
	}

	// check constraints
	// excluded day
	day := p.dayNumberForDate(starttime)
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
		roomnames, err := p.PlannedRoomNamesInSlot(ctx, starttime)
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
			return false, fmt.Errorf("room %s not found at %s", *roomName, starttime.Format("02.01. 15:04"))
		}
	}

	return p.dbClient.AddPrePlannedInvigilation(ctx, &model.PrePlannedInvigilation{
		InvigilatorID: invigilatorID,
		Starttime:     &starttime,
		RoomName:      roomName,
		IsReserve:     isReserve,
	})
}

func (p *Plexams) PrePlannedInvigilations(ctx context.Context) ([]*model.PrePlannedInvigilation, error) {
	return p.dbClient.PrePlannedInvigilations(ctx)
}

// RemovePrePlannedInvigilation removes a pre-planned invigilation (key:
// starttime/roomName; roomName nil = the reserve). If a live invigilation at that
// start time was marked pre-planned (via PrePlanInvigilationInSlot), that flag is
// cleared too. Errors if no such pre-planned invigilation exists.
func (p *Plexams) RemovePrePlannedInvigilation(ctx context.Context, starttime time.Time, roomName *string) (bool, error) {
	removed, err := p.dbClient.RemovePrePlannedInvigilationAt(ctx, starttime, roomName)
	if err != nil {
		return false, err
	}
	if !removed {
		room := "reserve"
		if roomName != nil {
			room = *roomName
		}
		return false, fmt.Errorf("no pre-planned invigilation for %s at %s", room, starttime.Format("02.01. 15:04"))
	}
	// best-effort: clear the pre-planned flag on the live invigilation, if any.
	if err := p.dbClient.SetInvigilationPrePlannedAt(ctx, starttime, roomName, false); err != nil {
		log.Debug().Err(err).Time("starttime", starttime).Msg("no live invigilation to unmark as pre-planned")
	}
	return true, nil
}

// PrePlanInvigilationInSlot promotes the invigilation currently planned for a
// room (roomName != nil) or the reserve (roomName == nil) at a start time to a
// pre-planned, fixed assignment. It looks up the assigned invigilator, stores a
// pre-planned invigilation for them and marks the live invigilation as
// pre-planned so the GUI can show it.
func (p *Plexams) PrePlanInvigilationInSlot(ctx context.Context, starttime time.Time, roomName *string) (bool, error) {
	room := "reserve"
	if roomName != nil {
		room = *roomName
	}

	teacher, err := p.dbClient.GetInvigilatorAt(ctx, room, starttime)
	if err != nil {
		return false, err
	}
	if teacher == nil {
		return false, fmt.Errorf("no invigilation for %s at %s to pre-plan", room, starttime.Format("02.01. 15:04"))
	}

	ok, err := p.PreAddInvigilation(ctx, teacher.ID, starttime, roomName)
	if err != nil || !ok {
		return false, err
	}

	if err := p.dbClient.SetInvigilationPrePlannedAt(ctx, starttime, roomName, true); err != nil {
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
