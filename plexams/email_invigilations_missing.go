package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
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
func (p *Plexams) SendEmailInvigilationReqMissing(ctx context.Context, run bool, reporter Reporter) error {
	reporter.Step("checking invigilators with missing requirements")
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
		reporter.StopProgress("no invigilators with missing requirements")
		return nil
	}

	reporter.Printf("%d invigilators with missing requirements", len(missing))

	sent := 0
	for _, invigilator := range missing {
		err := p.sendEmailInvigilationReqMissing(ctx, invigilator, run, reporter)
		if err != nil {
			log.Error().Err(err).Str("teacher", invigilator.Teacher.Shortname).
				Msg("cannot send email about missing invigilator requirements")
		} else {
			sent++
		}
	}

	reporter.StopProgress(fmt.Sprintf("sent %d of %d emails", sent, len(missing)))
	return nil
}

func (p *Plexams) sendEmailInvigilationReqMissing(ctx context.Context, invigilator *model.Invigilator, run bool, reporter Reporter) error {
	teacher := invigilator.Teacher

	reporter.Step(fmt.Sprintf("sending email about missing invigilator requirements to %s", teacher.Fullname))

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

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/emailBaseHTML.tmpl", "tmpl/invigilationMissingEmailHTML.tmpl")
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

	if err := p.sendMail(run, []string{teacher.Email}, nil, subject, bufText.Bytes(), bufHTML.Bytes(), nil, true); err != nil {
		reporter.Warnf("error while sending email to %s", teacher.Fullname)
		return err
	}

	reporter.Printf("  ✓ sent to %s (%d minutes) %s", teacher.Fullname, minutes, p.recipientInfo(run, teacher.Email))
	return nil
}
