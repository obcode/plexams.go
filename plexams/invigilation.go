package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
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
		return errNoExamsAt(starttime)
	}

	// check constraints
	// excluded day
	day := p.dayNumberForDate(starttime)
	for _, excludedDay := range invigilator.Requirements.ExcludedDays {
		if day == excludedDay {
			return apierr.Newf(apierr.CodeInvalidInput,
				"%s hat am %s einen Sperrtag und kann dort keine Aufsicht übernehmen. Wähle eine andere Person.",
				invigilator.Teacher.Fullname, dayLabelDE(starttime))
		}
	}
	// no exam in same slot
	for _, examInSlot := range examsInSlot {
		if examInSlot.ZpaExam.MainExamerID == invigilator.Teacher.ID {
			return apierr.Newf(apierr.CodeInvalidInput,
				"%s hat am %s eine eigene Prüfung und kann zu dieser Zeit keine Aufsicht übernehmen. Wähle eine andere Person.",
				invigilator.Teacher.Fullname, slotLabelDE(starttime))
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
		return false, errNoExamsAt(starttime)
	}

	// check constraints
	// excluded day
	day := p.dayNumberForDate(starttime)
	for _, excludedDay := range invigilator.Requirements.ExcludedDays {
		if day == excludedDay {
			return false, apierr.Newf(apierr.CodeInvalidInput,
				"%s hat am %s einen Sperrtag und kann dort nicht vorgeplant werden. Wähle eine andere Person.",
				invigilator.Teacher.Fullname, dayLabelDE(starttime))
		}
	}
	// no own exam in same slot
	for _, examInSlot := range examsInSlot {
		if examInSlot.ZpaExam.MainExamerID == invigilator.Teacher.ID {
			return false, apierr.Newf(apierr.CodeInvalidInput,
				"%s hat am %s eine eigene Prüfung und kann zu dieser Zeit nicht als Aufsicht vorgeplant werden. Wähle eine andere Person.",
				invigilator.Teacher.Fullname, slotLabelDE(starttime))
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
			return false, apierr.Newf(apierr.CodeInvalidInput,
				"Raum %s ist am %s für keine Prüfung eingeplant; dort kann niemand vorgeplant werden. Lade die Seite neu, falls sich die Raumplanung geändert hat.",
				*roomName, slotLabelDE(starttime))
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
		return false, apierr.Newf(apierr.CodeNotFound,
			"Für %s am %s gibt es keine vorgeplante Aufsicht; vermutlich wurde sie inzwischen entfernt. Lade die Seite neu.",
			invigilationPlaceDE(roomName), slotLabelDE(starttime))
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
		return false, apierr.Newf(apierr.CodeNotFound,
			"Für %s am %s ist niemand eingeteilt, also gibt es nichts zu fixieren. Plane stattdessen eine Person vor.",
			invigilationPlaceDE(roomName), slotLabelDE(starttime))
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
		return nil, apierr.Newf(apierr.CodeNotFound,
			"%s steht nicht in der Liste der Aufsichtspersonen dieses Semesters. Prüfe unter „Aufsichts-Constraints“ und „Dauerhaft keine Aufsicht“, ob die Person von Aufsichten ausgenommen ist.",
			p.teacherNames(ctx)(invigilatorID))
	}

	return invigilator, nil
}

// errNoExamsAt rejects an invigilation at a time without exams.
func errNoExamsAt(starttime time.Time) error {
	return apierr.Newf(apierr.CodeInvalidInput,
		"Am %s findet keine Prüfung statt, dort wird keine Aufsicht gebraucht.", slotLabelDE(starttime))
}
