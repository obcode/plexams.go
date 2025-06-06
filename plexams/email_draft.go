package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/jordan-wright/email"
	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) SendEmailDraft(run bool) error {
	err := p.sendEmailDraftZPA(run)
	if err != nil {
		log.Error().Err(err).Msg("cannot send email draft for ZPA")
		return err
	}
	err = p.sendEmailDraftFS(run)
	if err != nil {
		log.Error().Err(err).Msg("cannot send email draft for FS")
		return err
	}
	return nil
}

func (p *Plexams) sendEmailDraftZPA(run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email announcing the draft plan in ZPA")),
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

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/draftEmailZPA.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/draftEmailZPAHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Vorläufiger Prüfungsplan  - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	err = spinner.Stop()

	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	var to []string
	if run {
		to = []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas}
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

func (p *Plexams) sendEmailDraftFS(run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email announcing the draft plan to FS")),
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

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/draftEmailFS.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/draftEmailFSHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Vorläufiger Prüfungsplan - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	err = spinner.Stop()

	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	var to []string
	if run {
		to = []string{p.semesterConfig.Emails.Fs}
	} else {
		to = []string{"galority@gmail.com"}
	}

	// Generate the PDF draft
	bufMD, err := p.DraftFSBytes(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot generate PDF draft")
		return err
	}

	attachments := []*email.Attachment{
		{
			Filename: fmt.Sprintf("%s_Vorlaeufiger_Pruefungsplan_FK07_%s.pdf",
				time.Now().Format("2006-01-02"),
				string(bytes.ReplaceAll([]byte(p.semester), []byte(" "), []byte("_")))),
			ContentType: "application/pdf",
			Header:      map[string][]string{},
			Content:     bufMD,
			HTMLRelated: false,
		},
	}

	return p.sendMail(to,
		nil,
		subject,
		bufText.Bytes(),
		bufHTML.Bytes(),
		attachments,
		false,
	)
}
