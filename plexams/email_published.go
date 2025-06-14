package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) SendEmailPublished(ctx context.Context, run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email announcing published exams")),
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

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/publishedEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/publishedEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Prüfungsplan veröffentlicht",
		p.semester)

	err = spinner.Stop()

	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	var to []string
	if run {
		to = []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas, p.semesterConfig.Emails.Fs}
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
