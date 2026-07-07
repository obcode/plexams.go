package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
)

// buildLbaRepeaterExams fetches the planned exams and shapes them into the LBA-BA overview
// via email.BuildLbaRepeaterExams, resolving examers and per-room invigilators on demand.
func (p *Plexams) buildLbaRepeaterExams(ctx context.Context) ([]*email.LbaRepeaterExam, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return nil, err
	}
	examer := func(id int) *model.Teacher {
		t, err := p.GetTeacher(ctx, id)
		if err != nil {
			return nil
		}
		return t
	}
	invigilatorForRoom := func(room string, start time.Time) *model.Teacher {
		inv, err := p.GetInvigilatorForRoom(ctx, room, start)
		if err != nil {
			return nil
		}
		return inv
	}
	return email.BuildLbaRepeaterExams(plannedExams, examer, invigilatorForRoom), nil
}

// SendEmailLbaRepeaters sends the Lehrbeauftragten-Beauftragte:r (emails.lbaba)
// an overview of all repeat exams of LBAs that I planned — only dates and
// invigilations — so they know whom to remind. Answerable by email (no JIRA).
// Send-once (condLbaRepeatersSent).
func (p *Plexams) SendEmailLbaRepeaters(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condLbaRepeatersSent, run); err != nil {
		return err
	}
	reporter.Step("collecting LBA repeat exams")

	exams, err := p.buildLbaRepeaterExams(ctx)
	if err != nil {
		return err
	}
	if len(exams) == 0 {
		reporter.StopProgress("no LBA repeat exams planned by me, nothing to send")
		return nil
	}

	data := &email.LbaRepeaterEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Exams:        exams,
	}

	// the affected invigilators go into Cc
	ccSet := make(map[string]bool)
	for _, exam := range exams {
		for _, inv := range exam.Invigilators {
			if inv.Email != "" {
				ccSet[inv.Email] = true
			}
		}
	}
	cc := make([]string, 0, len(ccSet))
	for e := range ccSet {
		cc = append(cc, e)
	}
	sort.Strings(cc)

	text, html, err := p.mailRenderer().Render("lbaRepeaterEmail.md.tmpl", false, data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Wiederholungsprüfungen von Lehrbeauftragten", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Lbaba}, cc, subject, text, html, nil, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condLbaRepeatersSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s (%d exams)", p.recipientInfo(run, p.semesterConfig.Emails.Lbaba), len(exams)))
	return nil
}
