package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/jordan-wright/email"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
)

type CoverMailData struct {
	Teacher       *model.Teacher
	PlanerName    string
	GeneratorName string
}

func (p *Plexams) SendCoverPagesMails(ctx context.Context, run bool) error {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return err
	}

	examerIDs := set.NewSet[int]()
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		examerIDs.Add(exam.ZpaExam.MainExamerID)
	}

	for examerID := range examerIDs.Iter() {
		p.SendCoverPageMail(ctx, examerID, run) //nolint:errcheck
	}

	return nil
}

func (p *Plexams) SendCoverPageMail(ctx context.Context, examerID int, run bool) error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" sending email with cover pages for %4d"), examerID),
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

	teacher, err := p.GetTeacher(ctx, examerID)
	if err != nil {
		log.Debug().Err(err).Msg("cannot get teacher by ID")
		return err
	}

	dir := viper.GetString("coverPages.dir")
	prefix := viper.GetString("coverPages.prefix")
	filename := fmt.Sprintf("%s/%s%d.pdf", dir, prefix, examerID)

	pdfData, err := os.ReadFile(filename)
	if err != nil {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red(" %s: file not found: %s"),
			aurora.Magenta(teacher.Fullname), aurora.Magenta(filename)))
		spinner.StopFail() //nolint:errcheck
		return err
	}

	coverMailData := &CoverMailData{
		PlanerName:    p.planer.Name,
		Teacher:       teacher,
		GeneratorName: "Edda Eich-Söllner",
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/coverPageEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, coverMailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/coverPageEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, coverMailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Deckblätter für Ihre Prüfungen",
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
		[]*email.Attachment{{
			Filename:    strings.ReplaceAll(fmt.Sprintf("%s_Deckblaetter_Pruefungen_%s.pdf", p.semester, teacher.Fullname), " ", "_"),
			ContentType: "application/pdf",
			Header:      map[string][]string{},
			Content:     pdfData,
			HTMLRelated: false,
		}},
		true,
	)

	if err != nil {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red(" error while sending email to %s"), teacher.Fullname))
		return err
	}

	spinner.StopMessage(aurora.Sprintf(aurora.Cyan(" successfully send to %s"), teacher.Fullname))

	err = spinner.Stop()

	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	return nil
}
