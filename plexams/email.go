package plexams

import (
	"crypto/tls"
	"embed"
	"fmt"
	"net/smtp"
	"net/textproto"

	// TODO: Ersetzen durch github.com/wneessen/go-mail

	"github.com/jordan-wright/email"
)

//go:embed tmpl/constraintsEmail.tmpl
//go:embed tmpl/constraintsEmailHTML.tmpl
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
//go:embed tmpl/preparedEmail.tmpl
//go:embed tmpl/preparedEmailHTML.tmpl
//go:embed tmpl/publishedEmailExams.tmpl
//go:embed tmpl/publishedEmailExamsHTML.tmpl
//go:embed tmpl/publishedEmailRooms.tmpl
//go:embed tmpl/publishedEmailRoomsHTML.tmpl
//go:embed tmpl/publishedEmailInvigilations.tmpl
//go:embed tmpl/publishedEmailInvigilationsHTML.tmpl
//go:embed tmpl/invigilationEmail.tmpl
//go:embed tmpl/invigilationEmailHTML.tmpl
//go:embed tmpl/unplannedExamEmail.tmpl
//go:embed tmpl/unplannedExamEmailHTML.tmpl

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

func (p *Plexams) sendMail(to []string, cc []string, subject string, text []byte, html []byte, attachments []*email.Attachment, noreply bool) error {
	e := &email.Email{
		To:          to,
		Cc:          cc,
		Bcc:         []string{p.planer.Email},
		From:        fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject:     subject,
		Text:        text,
		HTML:        html,
		Headers:     textproto.MIMEHeader{},
		Attachments: attachments,
	}

	if noreply {
		e.ReplyTo = []string{"obraun+noreply@hm.edu"}
	}

	err := e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})

	return err
}
