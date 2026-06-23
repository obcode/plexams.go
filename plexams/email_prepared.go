package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/jordan-wright/email"
)

func (p *Plexams) SendEmailPrepared(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condExamsPrepared, run); err != nil {
		return err
	}
	reporter.Step("sending email announcing prepared exams and constraints")

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.New("preparedEmail.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/preparedEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	bufHTML, err := p.renderMailHTML("tmpl/preparedEmailHTML.tmpl", true, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Informationen zu den zu planenden Prüfungen und Besonderheiten - Rückmeldungen ASAP",
		p.semester)

	realRecipients := []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas, p.semesterConfig.Emails.LbasLastSemester}
	realRecipients = append(realRecipients, p.semesterConfig.Emails.AdditionalExamer...)

	examsToPlan, err := p.generateExamsToPlanBuffer(ctx)
	if err != nil {
		panic(err)
	}
	constraints, err := p.constraintsBuffer(ctx)
	if err != nil {
		panic(err)
	}

	attachments := []*email.Attachment{
		{
			Filename:    "ExamsToPlan.pdf",
			ContentType: "text/pdf; charset=\"utf-8\"",
			Header:      map[string][]string{},
			Content:     examsToPlan.Bytes(),
			HTMLRelated: false,
		},
		{
			Filename:    "Constraints.pdf",
			ContentType: "text/pdf; charset=\"utf-8\"",
			Header:      map[string][]string{},
			Content:     constraints.Bytes(),
			HTMLRelated: false,
		},
	}

	if err := p.sendMail(run, realRecipients, nil, subject, bufText.Bytes(), bufHTML, attachments, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condExamsPrepared)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, realRecipients...)))
	return nil
}
