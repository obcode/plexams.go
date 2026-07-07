package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
	"github.com/rs/zerolog/log"
)

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
	examsInSlot := make(map[time.Time][]*model.PlannedExam)
	getExamsInSlot := func(start time.Time) []*model.PlannedExam {
		if exams, ok := examsInSlot[start]; ok {
			return exams
		}
		exams, err := p.ExamsAt(ctx, start)
		if err != nil {
			log.Error().Err(err).Time("starttime", start).Msg("cannot get exams in slot")
		}
		examsInSlot[start] = exams
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
			if exam.PlanEntry != nil && exam.PlanEntry.Starttime != nil {
				getExamsInSlot(*exam.PlanEntry.Starttime)
			}
		}

		exams := make([]*email.PublishedRoomsExam, 0, len(plannedExams))
		for _, exam := range plannedExams {
			if e := email.BuildPublishedRoomsExam(exam, examsInSlot, examerShort); e != nil {
				exams = append(exams, e)
			}
		}
		if len(exams) == 0 {
			continue // examer has no exams with rooms
		}
		email.SortPublishedRoomsExams(exams)

		data := &email.PublishedRoomsEmail{
			Teacher:    examer,
			PlanerName: p.planer.Name,
			Exams:      exams,
		}

		text, html, err := p.mailRenderer().Render("publishedRoomsPersonalEmail.md.tmpl", true, data)
		if err != nil {
			reporter.Warnf("%s: cannot render: %v", examer.Fullname, err)
			continue
		}

		if err := p.sendMail(run, []string{examer.Email}, nil, subject, text, html, nil, true); err != nil {
			reporter.Warnf("error while sending email to %s: %v", examer.Fullname, err)
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
