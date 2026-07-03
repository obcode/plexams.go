package plexams

import (
	"context"
	"fmt"
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
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	text, html, err := p.renderMarkdownEmail("invigilationEmail.md.tmpl", true, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Anforderungen an die Planung der Prüfungsaufsichten - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Profs}, nil, subject, text, html, nil, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condInvigilationsRequested)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Profs)))
	return nil
}
