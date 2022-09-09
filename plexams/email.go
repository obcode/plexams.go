package plexams

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"net/textproto"

	"github.com/jordan-wright/email"
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

func (p *Plexams) SendHandicapsMail(ctx context.Context) error {
	return nil
}

func (p *Plexams) SendHandicapsMailToMainExamer(ctx context.Context, personID int) error {
	handicapsEmail := &HandicapsEmail{
		MainExamer: "Prof. Dr. Hugo Egon Balder",
		Exams: []*HandicapExam{
			{
				AnCode:       123,
				Module:       "Vereinigte Programmierung",
				TypeExamFull: "kurzer Vortrag 180 Minuten",
				HandicapStudents: []*HandicapStudent{
					{
						Name:         "Rainer Maria Rilke",
						Compensation: "100% Verlängerung und zwei eigene Räume",
					},
					{
						Name:         "Marius Müller Westernhagen",
						Compensation: "3 Bläser und 2 Schlagzeuger",
					},
				},
			},
			{
				AnCode:       123,
				Module:       "Cancel Culture",
				TypeExamFull: "nix machen",
				HandicapStudents: []*HandicapStudent{
					{
						Name:         "Rudolf Kunze",
						Compensation: "1000 mal schreiben",
					},
					{
						Name:         "Ramona und Ramona",
						Compensation: "Sushi-Stäbchen und drei Schüsseln",
					},
				},
			},
		},
		PlanerName: "Oliver Braun",
	}

	tmpl, err := template.ParseFiles("tmpl/handicapEmail.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		log.Fatal(err)
	}

	tmpl, err = template.ParseFiles("tmpl/handicapEmailHTML.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		log.Fatal(err)
	}

	return p.sendMail([]string{"galority@gmail.com"},
		fmt.Sprintf("[Prüfungsplanung %s] Nachteilausgleiche für Ihre Prüfungen", p.semester),
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
