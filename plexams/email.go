package plexams

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"

	// TODO: Ersetzen durch github.com/wneessen/go-mail

	"github.com/jordan-wright/email"
	"github.com/rs/zerolog/log"
)

//go:embed tmpl/constraintsEmail.tmpl
//go:embed tmpl/constraintsEmailHTML.tmpl
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
//go:embed tmpl/invigilationEmail.tmpl
//go:embed tmpl/invigilationEmailHTML.tmpl
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

// Deprecated: rm me
func (p *Plexams) SendHandicapsMails(ctx context.Context, run bool) error {
	ntasByTeacher, err := p.NtasWithRegsByTeacher(ctx)
	if err != nil {
		return err
	}

	for _, nta := range ntasByTeacher {
		exams := make([]*HandicapExam, 0, len(nta.Exams))
		for _, exam := range nta.Exams {
			handicapStudents := make([]*HandicapStudent, 0, len(exam.Ntas))
			for _, ntaForExam := range exam.Ntas {
				handicapStudents = append(handicapStudents, &HandicapStudent{
					Name:         ntaForExam.Nta.Name,
					Compensation: ntaForExam.Nta.Compensation,
				})
			}

			exams = append(exams, &HandicapExam{
				AnCode:           exam.Exam.AnCode,
				Module:           exam.Exam.Module,
				TypeExamFull:     exam.Exam.ExamTypeFull,
				HandicapStudents: handicapStudents,
			})
		}

		var to []string
		if run {
			to = []string{nta.Teacher.Email}
		} else {
			to = []string{"galority@gmail.com"}
		}

		err = p.SendHandicapsMailToMainExamer(ctx, to, &HandicapsEmail{
			MainExamer: nta.Teacher.Fullname,
			Exams:      exams,
			PlanerName: p.planer.Name,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Deprecated: rm me
func (p *Plexams) SendHandicapsMailToMainExamer(ctx context.Context, to []string, handicapsEmail *HandicapsEmail) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/handicapEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/handicapEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(to,
		nil,
		fmt.Sprintf("[Prüfungsplanung %s] Nachteilausgleich(e) für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		false,
	)
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

	if err != nil {
		return err
	}

	return p.Log(context.Background(), fmt.Sprintf("send email to %s", to), string(text))
}
