package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/rs/zerolog/log"
)

func (p *Plexams) SendEmailDraft(run bool, reporter Reporter) error {
	ctx := context.Background()
	if err := p.emailSendAllowed(ctx, condDraftSent, run); err != nil {
		return err
	}
	err := p.sendEmailDraftZPA(run, reporter)
	if err != nil {
		log.Error().Err(err).Msg("cannot send email draft for ZPA")
		return err
	}
	err = p.sendEmailDraftFS(run, reporter)
	if err != nil {
		log.Error().Err(err).Msg("cannot send email draft for FS")
		return err
	}
	if run {
		p.markCondition(ctx, condDraftSent)
	}
	return nil
}

func (p *Plexams) sendEmailDraftZPA(run bool, reporter Reporter) error {
	reporter.Step("sending email announcing the draft plan in ZPA")

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.New("draftEmailZPA.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/draftEmailZPA.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	bufHTML, err := p.renderMailHTML("tmpl/draftEmailZPAHTML.tmpl", true, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Vorläufiger Prüfungsplan  - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	realTo := []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas, p.semesterConfig.Emails.LbasLastSemester}
	realTo = append(realTo, p.semesterConfig.Emails.AdditionalExamer...)

	if err := p.sendMail(run, realTo, nil, subject, bufText.Bytes(), bufHTML, nil, true); err != nil {
		return err
	}
	reporter.StopProgress(fmt.Sprintf("draft (ZPA) email sent to %s", p.recipientInfo(run, realTo...)))
	return nil
}

func (p *Plexams) sendEmailDraftFS(run bool, reporter Reporter) error {
	reporter.Step("sending email announcing the draft plan to FS")

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

	bufHTML, err := p.renderMailHTML("tmpl/draftEmailFSHTML.tmpl", false, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Vorläufiger Prüfungsplan - Rückmeldung bis spätestens %s",
		p.semester, feedbackDate)

	// Generate the PDF draft
	bufMD, err := p.DraftFSBytes(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot generate PDF draft")
		return err
	}

	attachments := []*mailAttachment{
		{
			Filename: fmt.Sprintf("%s_Vorlaeufiger_Pruefungsplan_FK07_%s.pdf",
				time.Now().Format("2006-01-02"),
				string(bytes.ReplaceAll([]byte(p.semester), []byte(" "), []byte("_")))),
			ContentType: "application/pdf",
			Content:     bufMD,
		},
	}

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Fs}, nil, subject, bufText.Bytes(), bufHTML, attachments, false); err != nil {
		return err
	}
	reporter.StopProgress(fmt.Sprintf("draft (FS) email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Fs)))
	return nil
}
