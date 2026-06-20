package plexams

import (
	"context"
	"fmt"
	"time"

	ical "github.com/arran4/golang-ical"
	set "github.com/deckarep/golang-set/v2"
)

// InvigilatorICS builds an iCalendar (as bytes) with the appointments of one
// invigilator: their own exams (Eigenaufsicht when they supervise it themselves,
// otherwise Fremdaufsicht), their room invigilations (Aufsicht) and their
// reserve invigilations (Reserve) — the same items shown in the plan image.
func (p *Plexams) InvigilatorICS(ctx context.Context, invigilatorID int) ([]byte, error) {
	cal := ical.NewCalendar()
	cal.SetMethod(ical.MethodRequest)
	cal.SetProductId(fmt.Sprintf("-//Plexams Invigilation ICS//%s", p.semester))

	uid := 0
	addEvent := func(start time.Time, durationMinutes int, summary string) error {
		uid++
		ev := cal.AddEvent(fmt.Sprintf("%s-invig-%d-%d", p.semester, invigilatorID, uid))
		ev.SetSummary(summary)
		ev.SetStartAt(start)
		return ev.SetDuration(time.Duration(durationMinutes) * time.Minute)
	}

	allInvigs, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		return nil, err
	}

	// slots where the invigilator supervises their own exam, so the exam event
	// below can be labelled Eigenaufsicht instead of Fremdaufsicht.
	selfSlots := set.NewSet[[2]int]()
	for _, inv := range allInvigs {
		if inv.InvigilatorID != invigilatorID || inv.Slot == nil {
			continue
		}
		start := inv.Slot.Starttime
		switch {
		case inv.IsSelfInvigilation:
			selfSlots.Add([2]int{inv.Slot.DayNumber, inv.Slot.SlotNumber})
			// represented by the exam event (Eigenaufsicht) below
		case inv.IsReserve:
			if err := addEvent(start, inv.Duration, "Reserve"); err != nil {
				return nil, err
			}
		default:
			room := ""
			if inv.RoomName != nil {
				room = *inv.RoomName
			}
			if err := addEvent(start, inv.Duration, fmt.Sprintf("Aufsicht: %s", room)); err != nil {
				return nil, err
			}
		}
	}

	exams, err := p.GeneratedExamsForExamer(ctx, invigilatorID)
	if err != nil {
		return nil, err
	}
	for _, exam := range exams {
		planEntry, err := p.dbClient.PlanEntry(ctx, exam.Ancode)
		if err != nil || planEntry == nil {
			continue
		}
		start := p.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
		kind := "Fremdaufsicht"
		if selfSlots.Contains([2]int{planEntry.DayNumber, planEntry.SlotNumber}) {
			kind = "Eigenaufsicht"
		}
		module := ""
		if exam.ZpaExam != nil {
			module = exam.ZpaExam.Module
		}
		if err := addEvent(start, exam.MaxDuration, fmt.Sprintf("Prüfung (%s): %d. %s", kind, exam.Ancode, module)); err != nil {
			return nil, err
		}
	}

	return []byte(cal.Serialize()), nil
}
