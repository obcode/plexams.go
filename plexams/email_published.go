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

func (p *Plexams) SendEmailPublishedExams(ctx context.Context, run bool) error {
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

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/publishedEmailExams.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/publishedEmailExamsHTML.tmpl")
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

func (p *Plexams) SendEmailPublishedRooms(ctx context.Context, run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email announcing published rooms")),
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

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/publishedEmailRooms.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/publishedEmailRoomsHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Räume veröffentlicht",
		p.semester)

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

type InvigilationsEmail struct {
	NoOfInvigilators    int
	InvigilationInRooms int
	ReserveInvigilation int
	OtherContributions  int
	TodoPerInvigilator  int
	MaxDeviation        int
	MinDeviation        int
	PlanerName          string
}

func (p *Plexams) SendEmailPublishedInvigilations(ctx context.Context, run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email announcing published invigilations")),
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

	invigilationTodos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		return err
	}

	maxDeviation, minDeviation := 0, 0

	for _, invigilator := range invigilationTodos.Invigilators {
		deviation := invigilator.Todos.TotalMinutes - invigilator.Todos.DoingMinutes
		if deviation > 0 {
			if deviation > maxDeviation {
				maxDeviation = deviation
			}
		} else {
			if deviation < minDeviation {
				minDeviation = deviation
			}
		}
	}

	contraintsEmailData := &InvigilationsEmail{
		PlanerName:          p.planer.Name,
		NoOfInvigilators:    invigilationTodos.InvigilatorCount,
		InvigilationInRooms: invigilationTodos.SumExamRooms,
		ReserveInvigilation: invigilationTodos.SumReserve,
		OtherContributions:  invigilationTodos.SumOtherContributions,
		TodoPerInvigilator:  invigilationTodos.TodoPerInvigilatorOvertimeCutted,
		MaxDeviation:        maxDeviation,
		MinDeviation:        -minDeviation,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/publishedEmailInvigilations.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/publishedEmailInvigilationsHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Aufsichtenplanung im ZPA verfügbar",
		p.semester)

	err = spinner.Stop()

	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	var to []string
	if run {
		to = []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Sekr}
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
