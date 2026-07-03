package plexams

import (
	"context"
	"fmt"
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

	text, html, err := p.renderMarkdownEmail("exahmEmail.md.tmpl", true, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := "[Prüfungsplanung nächstes Semester] Prüfungen mit EXaHM und SEB - Rückmeldung bis so schnell wie möglich"

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Profs}, nil, subject, text, html, nil, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condExahmRequested)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Profs)))
	return nil
}
