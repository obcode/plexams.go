package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"
)

type ConstraintsEmail struct {
	FromDate     string
	FromFK07Date string
	UntilDate    string
	FeedbackDate string
	PlanerName   string
}

func (p *Plexams) SendEmailConstraints(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condConstraintsRequested, run); err != nil {
		return err
	}
	reporter.Step("sending email asking for constraints")

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.New("constraintsEmail.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/constraintsEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	bufHTML, err := p.renderMailHTML("tmpl/constraintsEmailHTML.tmpl", true, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Besonderheiten für die Prüfungsplanung - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	realRecipients := []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas, p.semesterConfig.Emails.LbasLastSemester}
	realRecipients = append(realRecipients, p.semesterConfig.Emails.AdditionalExamer...)
	if err := p.sendMail(run, realRecipients, nil, subject, bufText.Bytes(), bufHTML, nil, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condConstraintsRequested)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, realRecipients...)))
	return nil
}
