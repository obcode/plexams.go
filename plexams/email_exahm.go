package plexams

import (
	"bytes"
	"context"
	"html/template"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

type ExahmEmail struct {
	PlanerName string
}

func (p *Plexams) SendEmailExaHM(ctx context.Context, run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email asking for exahm and seb exams")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error happend",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}
	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

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

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/exahmEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := "[Prüfungsplanung nächstes Semester] Prüfungen mit EXaHM und SEB - Rückmeldung bis so schnell wie möglich"

	err = spinner.Stop()

	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	var to []string
	if run {
		to = []string{p.semesterConfig.Emails.Profs}
	} else {
		to = []string{"galority@gmail.com"}
	}

	return p.sendMail(to,
		nil,
		subject,
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		true,
	)
}
