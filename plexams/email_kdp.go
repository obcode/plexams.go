package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jszwec/csvutil"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
)

// kdpExamerCC returns the sorted, de-duplicated emails of the examers of the
// EXaHM/SEB exams that have a plan entry — the CC of the KDP mail.
func (p *Plexams) kdpExamerCC(ctx context.Context, plannedExams []*model.PlannedExam) []string {
	examerEmails := make(map[string]bool)
	for _, exam := range plannedExams {
		if !email.IsExahmSeb(exam) || exam.PlanEntry == nil {
			continue
		}
		if teacher, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID); err == nil && teacher != nil && teacher.Email != "" {
			examerEmails[teacher.Email] = true
		}
	}
	ccEmails := make([]string, 0, len(examerEmails))
	for e := range examerEmails {
		ccEmails = append(ccEmails, e)
	}
	sort.Strings(ccEmails)
	return ccEmails
}

// SendEmailKdpExahm sends the KDP the overview of the EXaHM/SEB room planning,
// ordered by day/time and room (with a per-exam recap), plus a room-oriented CSV
// attachment. Answerable by email (no JIRA). Send-once (condKdpRoomsSent), after
// the room plan has been published.
func (p *Plexams) SendEmailKdpExahm(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condKdpRoomsSent, run); err != nil {
		return err
	}
	reporter.Step("collecting EXaHM/SEB room planning for the KDP")

	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return err
	}

	slots, csvRows := email.BuildKdp(plannedExams, p.getSlotTime)
	if len(slots) == 0 {
		reporter.StopProgress("no EXaHM/SEB rooms planned, nothing to send")
		return nil
	}
	data := &email.KdpEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Slots:        slots,
	}
	ccEmails := p.kdpExamerCC(ctx, plannedExams)

	csvBytes, err := csvutil.Marshal(csvRows)
	if err != nil {
		return err
	}

	text, html, err := p.mailRenderer().Render("kdpExahmEmail.md.tmpl", false, data)
	if err != nil {
		return err
	}

	attachments := []*mailAttachment{{
		Filename:    fmt.Sprintf("%s_EXaHM_SEB_Raeume.csv", strings.ReplaceAll(p.semester, " ", "_")),
		ContentType: "text/csv; charset=utf-8",
		Content:     csvBytes,
	}}

	subject := fmt.Sprintf("[Prüfungsplanung %s] EXaHM/SEB – Raumübersicht für das KDP", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Kdp}, ccEmails, subject, text, html, attachments, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condKdpRoomsSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Kdp)))
	return nil
}
