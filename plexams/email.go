package plexams

import (
	"crypto/tls"
	"embed"
	"fmt"
	"net/smtp"
	"net/textproto"
	"strings"

	// TODO: Ersetzen durch github.com/wneessen/go-mail

	"github.com/jordan-wright/email"
)

//go:embed tmpl/constraintsEmail.tmpl
//go:embed tmpl/constraintsEmailHTML.tmpl
//go:embed tmpl/exahmEmail.tmpl
//go:embed tmpl/exahmEmailHTML.tmpl
//go:embed tmpl/coverPageEmail.tmpl
//go:embed tmpl/coverPageEmailHTML.tmpl
//go:embed tmpl/draftEmailFS.tmpl
//go:embed tmpl/draftEmailFSHTML.tmpl
//go:embed tmpl/draftEmailZPA.tmpl
//go:embed tmpl/draftEmailZPAHTML.tmpl
//go:embed tmpl/generatedExamEmail.tmpl
//go:embed tmpl/generatedExamEmailHTML.tmpl
//go:embed tmpl/generatedExamMarkdown.tmpl
//go:embed tmpl/handicapEmail.tmpl
//go:embed tmpl/handicapEmailHTML.tmpl
//go:embed tmpl/handicapEmailPlanned.tmpl
//go:embed tmpl/handicapEmailPlannedHTML.tmpl
//go:embed tmpl/handicapEmailRoomAlone.tmpl
//go:embed tmpl/handicapEmailRoomAloneHTML.tmpl
//go:embed tmpl/newNTAEmail.tmpl
//go:embed tmpl/newNTAEmailHTML.tmpl
//go:embed tmpl/preparedEmail.tmpl
//go:embed tmpl/preparedEmailHTML.tmpl
//go:embed tmpl/publishedEmailExams.tmpl
//go:embed tmpl/publishedEmailExamsHTML.tmpl
//go:embed tmpl/publishedRoomsPersonalEmail.tmpl
//go:embed tmpl/publishedRoomsPersonalEmailHTML.tmpl
//go:embed tmpl/publishedEmailInvigilations.tmpl
//go:embed tmpl/publishedEmailInvigilationsHTML.tmpl
//go:embed tmpl/publishedInvigilationPersonalEmail.tmpl
//go:embed tmpl/publishedInvigilationPersonalEmailHTML.tmpl
//go:embed tmpl/invigilationEmail.tmpl
//go:embed tmpl/invigilationEmailHTML.tmpl
//go:embed tmpl/invigilationMissingEmail.tmpl
//go:embed tmpl/invigilationMissingEmailHTML.tmpl
//go:embed tmpl/unplannedExamEmail.tmpl
//go:embed tmpl/unplannedExamEmailHTML.tmpl
//go:embed tmpl/roomRequestEmail.tmpl
//go:embed tmpl/roomRequestEmailHTML.tmpl

var emailTemplates embed.FS

func (p *Plexams) SendTestMail() error {
	e := &email.Email{
		To:      []string{p.planer.Email},
		From:    fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject: "Awesome Subject",
		Text:    []byte("Text Body is, of course, supported!"),
		HTML:    []byte("<h1>Fancy HTML is supported, too!</h1>"),
		Headers: textproto.MIMEHeader{},
	}

	return e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})
}

// dryRunRecipient is the address all dry-run mails go to: the configured test
// address (smtp.testmail), or the planner's address when none is configured.
func (p *Plexams) dryRunRecipient() string {
	if p.email.testMail != "" {
		return p.email.testMail
	}
	return p.planer.Email
}

// recipientInfo describes a send for progress/log output. On a dry run it makes
// explicit that the mail went to the dry-run address only and lists who it would
// have reached, so the output never looks like a real send to real recipients.
func (p *Plexams) recipientInfo(run bool, recipients ...string) string {
	if run {
		return fmt.Sprintf("%v", recipients)
	}
	return fmt.Sprintf("PROBEVERSAND an %s (echte Empfänger wären: %v)", p.dryRunRecipient(), recipients)
}

func (p *Plexams) sendMail(run bool, to []string, cc []string, subject string, text []byte, html []byte, attachments []*email.Attachment, noreply bool) error {
	actualTo := to
	actualCc := cc
	bcc := []string{p.planer.Email}

	if !run {
		// Probeversand: alles geht an die Test-Adresse. Der Betreff wird mit
		// den echten Empfängern (To + Cc) präfixt, damit klar ist, an wen die
		// E-Mail tatsächlich versandt worden wäre.
		realRecipients := append(append([]string{}, to...), cc...)
		if len(realRecipients) > 0 {
			subject = fmt.Sprintf("[Probeversand → %s] %s", strings.Join(realRecipients, ", "), subject)
		} else {
			subject = fmt.Sprintf("[Probeversand] %s", subject)
		}
		actualTo = []string{p.dryRunRecipient()}
		actualCc = nil
		bcc = nil
	}

	e := &email.Email{
		To:          actualTo,
		Cc:          actualCc,
		Bcc:         bcc,
		From:        fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject:     subject,
		Text:        text,
		HTML:        html,
		Headers:     textproto.MIMEHeader{},
		Attachments: attachments,
	}

	if noreply {
		replyTo := "noreply@hm.edu"
		if localPart, domain, ok := strings.Cut(p.planer.Email, "@"); ok && localPart != "" && domain != "" {
			replyTo = fmt.Sprintf("%s+pruefungsplanung_noreply@%s", localPart, domain)
		}
		e.ReplyTo = []string{replyTo}
	}

	err := e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})

	return err
}
