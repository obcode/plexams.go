package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// InvigilatorICS builds an iCalendar (as bytes) with the appointments of one
// invigilator: their own exams (Eigenaufsicht when self-supervised, otherwise
// Fremdaufsicht), their room invigilations (Aufsicht) and their reserve
// invigilations (Reserve) — the items shown in the plan image. Each event's
// description carries the relevant rooms, invigilators and NTAs.
func (p *Plexams) InvigilatorICS(ctx context.Context, invigilatorID int) ([]byte, error) {
	cal := ical.NewCalendar()
	cal.SetMethod(ical.MethodRequest)
	cal.SetProductId(fmt.Sprintf("-//Plexams Invigilation ICS//%s", p.semester))

	// caches to avoid repeated DB lookups
	examsInSlot := make(map[[2]int][]*model.PlannedExam)
	getExams := func(day, slot int) []*model.PlannedExam {
		key := [2]int{day, slot}
		if exams, ok := examsInSlot[key]; ok {
			return exams
		}
		exams, err := p.ExamsInSlot(ctx, day, slot)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("ics: cannot get exams in slot")
			exams = nil
		}
		plannedByMe := make([]*model.PlannedExam, 0, len(exams))
		for _, exam := range exams {
			if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
				continue
			}
			plannedByMe = append(plannedByMe, exam)
		}
		examsInSlot[key] = plannedByMe
		return plannedByMe
	}
	invigName := func(room string, day, slot int) string {
		t, err := p.GetInvigilatorInSlot(ctx, room, day, slot)
		if err != nil || t == nil {
			return "—"
		}
		return t.Shortname
	}

	uid := 0
	addEvent := func(start time.Time, durationMinutes int, summary, description string) error {
		uid++
		ev := cal.AddEvent(fmt.Sprintf("%s-invig-%d-%d", p.semester, invigilatorID, uid))
		ev.SetSummary(summary)
		if description != "" {
			ev.SetDescription(description)
		}
		ev.SetStartAt(start)
		return ev.SetDuration(time.Duration(durationMinutes) * time.Minute)
	}

	// roomLine renders one planned room with optional invigilator and its NTA.
	roomLine := func(exam *model.PlannedExam, room *model.PlannedRoom, withInvigilator bool) string {
		var b strings.Builder
		fmt.Fprintf(&b, "%s (%d min, %d Stud.)", room.RoomName, room.Duration, len(room.StudentsInRoom))
		if withInvigilator {
			fmt.Fprintf(&b, ", Aufsicht: %s", invigName(room.RoomName, room.Day, room.Slot))
		}
		if nta := roomNTA(exam, room); nta != nil {
			alone := ""
			if nta.NeedsRoomAlone {
				alone = ", eigener Raum"
			}
			fmt.Fprintf(&b, ", NTA: %s (+%d%%%s)", nta.Name, nta.DeltaDurationPercent, alone)
		}
		return b.String()
	}

	// examRooms renders all real rooms of an exam, one per line.
	examRooms := func(exam *model.PlannedExam, withInvigilator bool, skipRoom string) []string {
		lines := make([]string, 0, len(exam.PlannedRooms))
		for _, room := range exam.PlannedRooms {
			if room.RoomName == noRoom || room.RoomName == skipRoom {
				continue
			}
			lines = append(lines, "  - "+roomLine(exam, room, withInvigilator))
		}
		return lines
	}

	examerShortCache := make(map[int]string)
	examerShort := func(exam *model.PlannedExam) string {
		if exam.ZpaExam == nil {
			return ""
		}
		id := exam.ZpaExam.MainExamerID
		if s, ok := examerShortCache[id]; ok {
			return s
		}
		s := exam.ZpaExam.MainExamer
		if t, err := p.GetTeacher(ctx, id); err == nil && t != nil && t.Shortname != "" {
			s = t.Shortname
		}
		examerShortCache[id] = s
		return s
	}

	examHeader := func(exam *model.PlannedExam) string {
		module := ""
		if exam.ZpaExam != nil {
			module = exam.ZpaExam.Module
		}
		if short := examerShort(exam); short != "" {
			return fmt.Sprintf("%d. %s (%s)", exam.Ancode, module, short)
		}
		return fmt.Sprintf("%d. %s", exam.Ancode, module)
	}

	// ---- invigilations: Aufsicht and Reserve (self is handled via the exam) ----
	allInvigs, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		return nil, err
	}
	selfRoomBySlot := make(map[[2]int]string)
	for _, inv := range allInvigs {
		if inv.InvigilatorID != invigilatorID || inv.Slot == nil {
			continue
		}
		day, slot := inv.Slot.DayNumber, inv.Slot.SlotNumber
		start := inv.Slot.Starttime

		switch {
		case inv.IsSelfInvigilation:
			if inv.RoomName != nil {
				selfRoomBySlot[[2]int{day, slot}] = *inv.RoomName
			}
			// the exam event below renders it as Eigenaufsicht

		case inv.IsReserve:
			// Reserve: all exams in the slot with rooms, invigilators and NTAs.
			var b strings.Builder
			b.WriteString("Prüfungen in diesem Slot:")
			for _, exam := range getExams(day, slot) {
				fmt.Fprintf(&b, "\n%s", examHeader(exam))
				for _, line := range examRooms(exam, true, "") {
					b.WriteString("\n" + line)
				}
			}
			if err := addEvent(start, inv.Duration, "Reserve", b.String()); err != nil {
				return nil, err
			}

		default:
			// Aufsicht: the exam in this room; title incl. exam, body: NTA in this
			// room + the other rooms (incl. NTAs) of that exam.
			room := ""
			if inv.RoomName != nil {
				room = *inv.RoomName
			}
			var ownExam *model.PlannedExam
			for _, exam := range getExams(day, slot) {
				for _, r := range exam.PlannedRooms {
					if r.RoomName == room {
						ownExam = exam
						break
					}
				}
				if ownExam != nil {
					break
				}
			}

			summary := fmt.Sprintf("Aufsicht: %s", room)
			var b strings.Builder
			if ownExam != nil {
				summary = fmt.Sprintf("Aufsicht: %s — %s", room, examHeader(ownExam))
				// NTA in this room
				for _, r := range ownExam.PlannedRooms {
					if r.RoomName == room {
						if nta := roomNTA(ownExam, r); nta != nil {
							alone := ""
							if nta.NeedsRoomAlone {
								alone = ", eigener Raum"
							}
							fmt.Fprintf(&b, "NTA in Ihrem Raum: %s (+%d%%%s)\n", nta.Name, nta.DeltaDurationPercent, alone)
						}
					}
				}
				other := examRooms(ownExam, false, room)
				if len(other) > 0 {
					b.WriteString("Weitere Räume der Prüfung:")
					for _, line := range other {
						b.WriteString("\n" + line)
					}
				}
			}
			if err := addEvent(start, inv.Duration, summary, strings.TrimRight(b.String(), "\n")); err != nil {
				return nil, err
			}
		}
	}

	// ---- own exams: Eigenaufsicht / Fremdaufsicht ----
	exams, err := p.PlannedExamsByExamer(ctx, invigilatorID)
	if err != nil {
		return nil, err
	}
	for _, exam := range exams {
		if exam.PlanEntry == nil {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		day, slot := exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber
		start := p.getSlotTime(day, slot)

		selfRoom, isSelf := selfRoomBySlot[[2]int{day, slot}]

		var b strings.Builder
		kind := "Fremdaufsicht"
		if isSelf {
			kind = "Eigenaufsicht"
			// Eigenaufsicht: your room and its NTAs.
			b.WriteString("Ihr Raum:")
			rendered := false
			for _, room := range exam.PlannedRooms {
				if room.RoomName == selfRoom {
					b.WriteString("\n  - " + roomLine(exam, room, false))
					rendered = true
				}
			}
			if !rendered {
				for _, line := range examRooms(exam, false, "") {
					b.WriteString("\n" + line)
				}
			}
		} else {
			// Fremdaufsicht: rooms with their invigilators and NTAs.
			b.WriteString("Räume und Aufsichten:")
			for _, line := range examRooms(exam, true, "") {
				b.WriteString("\n" + line)
			}
		}

		summary := fmt.Sprintf("Prüfung (%s): %s", kind, examHeader(exam))
		if err := addEvent(start, exam.MaxDuration, summary, b.String()); err != nil {
			return nil, err
		}
	}

	return []byte(cal.Serialize()), nil
}

// roomNTA returns the NTA assigned to a room (via NtaMtknr), or nil.
func roomNTA(exam *model.PlannedExam, room *model.PlannedRoom) *model.NTA {
	if room.NtaMtknr == nil {
		return nil
	}
	for _, nta := range exam.Ntas {
		if nta != nil && nta.Mtknr == *room.NtaMtknr {
			return nta
		}
	}
	return nil
}
