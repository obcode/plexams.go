package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// PublishedRoomsEmail is the data for one examer's "rooms published" email.
type PublishedRoomsEmail struct {
	Teacher    *model.Teacher
	PlanerName string
	Exams      []*publishedRoomsExam
}

type publishedRoomsExam struct {
	Ancode int
	Module string
	Date   string // e.g. "Mo, 13.07.2026"
	Time   string // e.g. "08:30"
	Rooms  []*publishedRoomsRoom
}

type publishedRoomsRoom struct {
	RoomName     string
	StudentCount int
	Duration     int
	Reserve      bool
	NTA          string   // formatted NTA note, "" if none
	SharedWith   []string // other exams using the same room in the same slot
}

// buildPublishedRoomsExam renders one exam (rooms + NTAs + co-usage) for the
// examer email. Returns nil if the exam has no real (non-"No Room") rooms.
func (p *Plexams) buildPublishedRoomsExam(ctx context.Context, exam *model.PlannedExam,
	examsInSlot map[[2]int][]*model.PlannedExam, examerShort func(*model.PlannedExam) string,
) *publishedRoomsExam {
	if exam.PlanEntry == nil {
		return nil
	}
	day, slot := exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber
	start := p.getSlotTime(day, slot)

	rooms := make([]*publishedRoomsRoom, 0, len(exam.PlannedRooms))
	for _, room := range exam.PlannedRooms {
		if room.RoomName == noRoom {
			continue
		}
		ntaNote := ""
		if nta := roomNTA(exam, room); nta != nil {
			alone := ""
			if nta.NeedsRoomAlone {
				alone = ", eigener Raum"
			}
			ntaNote = fmt.Sprintf("%s (+%d%%%s)", nta.Name, nta.DeltaDurationPercent, alone)
		}

		// co-usage: other exams with a room of the same name in the same slot.
		shared := make([]string, 0)
		for _, other := range examsInSlot[[2]int{day, slot}] {
			if other.Ancode == exam.Ancode {
				continue
			}
			for _, r := range other.PlannedRooms {
				if r.RoomName != room.RoomName {
					continue
				}
				module := ""
				if other.ZpaExam != nil {
					module = other.ZpaExam.Module
				}
				shared = append(shared, fmt.Sprintf("%d. %s (%s): %d Stud.",
					other.Ancode, module, examerShort(other), len(r.StudentsInRoom)))
			}
		}

		rooms = append(rooms, &publishedRoomsRoom{
			RoomName:     room.RoomName,
			StudentCount: len(room.StudentsInRoom),
			Duration:     room.Duration,
			Reserve:      room.Reserve,
			NTA:          ntaNote,
			SharedWith:   shared,
		})
	}

	if len(rooms) == 0 {
		return nil
	}

	module := ""
	if exam.ZpaExam != nil {
		module = exam.ZpaExam.Module
	}
	return &publishedRoomsExam{
		Ancode: exam.Ancode,
		Module: module,
		Date:   fmt.Sprintf("%s, %s", weekdayShortDE[int(start.Weekday())], start.Format("02.01.2006")),
		Time:   start.Format("15:04"),
		Rooms:  rooms,
	}
}

// SendEmailPublishedRooms sends one individual "rooms published" email per examer
// with exams planned by me, listing the planned rooms of all their exams with
// student counts, reserve flag, the NTAs in the rooms and which other exams share
// the same room in the same slot (co-usage). Send-once (condRoomPlanPublished).
func (p *Plexams) SendEmailPublishedRooms(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condRoomPlanPublished, run); err != nil {
		return err
	}
	reporter.Step("preparing rooms-published emails")

	examers, err := p.ExamersWithExamsPlannedByMe(ctx)
	if err != nil {
		return err
	}

	// caches across all examers
	examsInSlot := make(map[[2]int][]*model.PlannedExam)
	getExamsInSlot := func(day, slot int) []*model.PlannedExam {
		key := [2]int{day, slot}
		if exams, ok := examsInSlot[key]; ok {
			return exams
		}
		exams, err := p.ExamsInSlot(ctx, day, slot)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("cannot get exams in slot")
		}
		examsInSlot[key] = exams
		return exams
	}
	shortCache := make(map[int]string)
	examerShort := func(exam *model.PlannedExam) string {
		if exam.ZpaExam == nil {
			return ""
		}
		id := exam.ZpaExam.MainExamerID
		if s, ok := shortCache[id]; ok {
			return s
		}
		s := exam.ZpaExam.MainExamer
		if t, err := p.GetTeacher(ctx, id); err == nil && t != nil && t.Shortname != "" {
			s = t.Shortname
		}
		shortCache[id] = s
		return s
	}

	textTmpl, err := template.ParseFS(emailTemplates, "tmpl/publishedRoomsPersonalEmail.tmpl")
	if err != nil {
		return err
	}
	htmlTmpl, err := template.ParseFS(emailTemplates, "tmpl/publishedRoomsPersonalEmailHTML.tmpl")
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Ihre Prüfungsräume", p.semester)

	sent := 0
	for _, examer := range examers {
		plannedExams, err := p.PlannedExamsByExamer(ctx, examer.ID)
		if err != nil {
			reporter.Warnf("%s: cannot get exams: %v", examer.Fullname, err)
			continue
		}

		// prime the slot cache so co-usage sees every exam in the slot
		for _, exam := range plannedExams {
			if exam.PlanEntry != nil {
				getExamsInSlot(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			}
		}

		exams := make([]*publishedRoomsExam, 0, len(plannedExams))
		for _, exam := range plannedExams {
			if e := p.buildPublishedRoomsExam(ctx, exam, examsInSlot, examerShort); e != nil {
				exams = append(exams, e)
			}
		}
		if len(exams) == 0 {
			continue // examer has no exams with rooms
		}
		sort.SliceStable(exams, func(i, j int) bool {
			if exams[i].Date != exams[j].Date {
				return exams[i].Date < exams[j].Date
			}
			return exams[i].Time < exams[j].Time
		})

		data := &PublishedRoomsEmail{
			Teacher:    examer,
			PlanerName: p.planer.Name,
			Exams:      exams,
		}

		bufText := new(bytes.Buffer)
		if err := textTmpl.Execute(bufText, data); err != nil {
			reporter.Warnf("%s: cannot render text: %v", examer.Fullname, err)
			continue
		}
		bufHTML := new(bytes.Buffer)
		if err := htmlTmpl.Execute(bufHTML, data); err != nil {
			reporter.Warnf("%s: cannot render html: %v", examer.Fullname, err)
			continue
		}

		if err := p.sendMail(run, []string{examer.Email}, nil, subject, bufText.Bytes(), bufHTML.Bytes(), nil, true); err != nil {
			reporter.Warnf("error while sending email to %s", examer.Fullname)
			continue
		}
		reporter.Printf("  ✓ %s", p.recipientInfo(run, examer.Email))
		sent++
	}

	if run {
		p.markCondition(ctx, condRoomPlanPublished)
	}
	reporter.StopProgress(fmt.Sprintf("sent %d rooms-published emails", sent))
	return nil
}
