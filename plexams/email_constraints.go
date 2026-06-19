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
	reporter.Step("sending email asking for constraints")

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/constraintsEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/constraintsEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Besonderheiten für die Prüfungsplanung - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	realRecipients := []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas, p.semesterConfig.Emails.LbasLastSemester}
	realRecipients = append(realRecipients, p.semesterConfig.Emails.AdditionalExamer...)
	to := p.mailTo(run, realRecipients...)

	if err := p.sendMail(to, nil, subject, bufText.Bytes(), bufHTML.Bytes(), nil, true); err != nil {
		return err
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %v", to))
	return nil
}
