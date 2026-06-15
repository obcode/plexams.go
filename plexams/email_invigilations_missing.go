package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

type InvigilationMissingMailData struct {
	Teacher    *model.Teacher
	Semester   string
	PlanerName string
	Minutes    int
}

// SendEmailInvigilationReqMissing sends an email to every invigilator who has
// not yet entered their invigilation requirements ("Anforderungen an die
// Planung der Prüfungsaufsichten") in the ZPA. They are warned that, without
// these requirements, they have to be planned with the full amount of
// invigilation duty (100%).
func (p *Plexams) SendEmailInvigilationReqMissing(ctx context.Context, run bool) error {
	invigilationTodos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilation todos")
		return err
	}

	missing := make([]*model.Invigilator, 0)
	for _, invigilator := range invigilationTodos.Invigilators {
		if invigilator.Requirements == nil || !invigilator.Requirements.FromZpa {
			missing = append(missing, invigilator)
		}
	}

	if len(missing) == 0 {
		fmt.Println(aurora.Sprintf(aurora.Red("no invigilators with missing requirements")))
		return nil
	}

	fmt.Println(aurora.Sprintf(aurora.Cyan("%d invigilators with missing requirements"), len(missing)))

	for _, invigilator := range missing {
		err := p.sendEmailInvigilationReqMissing(ctx, invigilator, run)
		if err != nil {
			log.Error().Err(err).Str("teacher", invigilator.Teacher.Shortname).
				Msg("cannot send email about missing invigilator requirements")
		}
	}

	return nil
}

func (p *Plexams) sendEmailInvigilationReqMissing(ctx context.Context, invigilator *model.Invigilator, run bool) error {
	teacher := invigilator.Teacher

	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Magenta(" sending email about missing invigilator requirements to %s"), teacher.Fullname),
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

	minutes := 0
	if invigilator.Todos != nil {
		minutes = invigilator.Todos.TotalMinutes
	}

	mailData := &InvigilationMissingMailData{
		Teacher:    teacher,
		Semester:   p.semester,
		PlanerName: p.planer.Name,
		Minutes:    minutes,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/invigilationMissingEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, mailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/invigilationMissingEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, mailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Fehlende Anforderungen an die Planung der Prüfungsaufsichten",
		p.semester)

	var to []string
	if run {
		to = []string{teacher.Email}
	} else {
		to = []string{"galority@gmail.com"}
	}

	err = p.sendMail(to,
		nil,
		subject,
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		true,
	)

	if err != nil {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red(" error while sending email to %s"), teacher.Fullname))
		spinner.StopFail() //nolint:errcheck
		return err
	}

	spinner.StopMessage(aurora.Sprintf(aurora.Cyan(" successfully send to %s (%d minutes)"), teacher.Fullname, minutes))

	err = spinner.Stop()
	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	return nil
}
