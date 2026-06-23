package plexams

import (
	"bytes"
	"context"
	"fmt"
	txttmpl "text/template"
)

// secretariatInvigEmail is the (minimal) data for the "invigilations published"
// note to the secretariat.
type secretariatInvigEmail struct {
	SemesterName string
	PlanerName   string
}

// SendEmailInvigilationsSecretariat sends the secretariat a short note that the
// invigilation planning is finished, everything is in ZPA and the plan may be
// posted. Answerable by email (no JIRA). Send-once (condInvigSecretariatSent),
// after the invigilation plan has been published.
func (p *Plexams) SendEmailInvigilationsSecretariat(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condInvigSecretariatSent, run); err != nil {
		return err
	}
	reporter.Step("sending invigilations-published note to the secretariat")

	data := &secretariatInvigEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
	}

	textTmpl, err := txttmpl.ParseFS(emailTemplates, "tmpl/invigilationsSecretariatEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	if err := textTmpl.Execute(bufText, data); err != nil {
		return err
	}

	bufHTML, err := p.renderMailHTML("tmpl/invigilationsSecretariatEmailHTML.tmpl", false, data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Prüfungsplan veröffentlicht – kann ausgehängt werden", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Sekr}, nil, subject, bufText.Bytes(), bufHTML, nil, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condInvigSecretariatSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Sekr)))
	return nil
}
