package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"
)

func (p *Plexams) SendEmailInvigilations(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condInvigilationsRequested, run); err != nil {
		return err
	}
	reporter.Step("sending email requesting invigilations constraints")

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/invigilationEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/invigilationEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Anforderungen an die Planung der Prüfungsaufsichten - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	to := p.mailTo(run, p.semesterConfig.Emails.Profs)

	if err := p.sendMail(to, nil, subject, bufText.Bytes(), bufHTML.Bytes(), nil, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condInvigilationsRequested)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %v", to))
	return nil
}
