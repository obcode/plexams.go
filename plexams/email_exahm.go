package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
)

type ExahmEmail struct {
	PlanerName string
}

func (p *Plexams) SendEmailExaHM(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condExahmRequested, run); err != nil {
		return err
	}
	reporter.Step("sending email asking for exahm and seb exams")

	contraintsEmailData := &ExahmEmail{
		PlanerName: p.planer.Name,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/exahmEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/emailBaseHTML.tmpl", "tmpl/exahmEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := "[Prüfungsplanung nächstes Semester] Prüfungen mit EXaHM und SEB - Rückmeldung bis so schnell wie möglich"

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Profs}, nil, subject, bufText.Bytes(), bufHTML.Bytes(), nil, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condExahmRequested)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Profs)))
	return nil
}
