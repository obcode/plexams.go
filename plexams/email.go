package plexams

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"

	"github.com/jordan-wright/email"
	"github.com/rs/zerolog/log"
)

type HandicapsEmail struct {
	MainExamer string
	Exams      []*HandicapExam
	PlanerName string
}

type HandicapExam struct {
	AnCode           int
	Module           string
	TypeExamFull     string
	HandicapStudents []*HandicapStudent
}

type HandicapStudent struct {
	Name         string
	Compensation string
}

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

func (p *Plexams) SendHandicapsMailToMainExamer(ctx context.Context, to []string, handicapsEmail *HandicapsEmail) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFiles("tmpl/handicapEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFiles("tmpl/handicapEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(to,
		fmt.Sprintf("[Prüfungsplanung %s] Nachteilausgleich(e) für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
	)
}

func (p *Plexams) sendMail(to []string, subject string, text []byte, html []byte) error {
	e := &email.Email{
		To:      to,
		Bcc:     []string{p.planer.Email},
		From:    fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject: subject,
		Text:    text,
		HTML:    html,
		Headers: textproto.MIMEHeader{},
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
