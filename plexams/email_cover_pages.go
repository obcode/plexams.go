package plexams

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type CoverMailData struct {
	Teacher       *model.Teacher
	PlanerName    string
	GeneratorName string
}

// coverPagePDF returns the cover-page PDF for an examer: first from the uploaded
// attachment store (kind cover-page, key = examer id), then falling back to the
// configured coverPages.dir so the CLI keeps working without an upload. The
// returned source is a short label for logging/streaming.
func (p *Plexams) coverPagePDF(ctx context.Context, examerID int) (data []byte, source string, err error) {
	att, err := p.GetEmailAttachment(ctx, AttachmentKindCoverPage, strconv.Itoa(examerID))
	if err != nil {
		log.Error().Err(err).Int("examerID", examerID).Msg("cannot read cover page from store")
	}
	if att != nil && len(att.Data) > 0 {
		return att.Data, "upload", nil
	}

	dir := viper.GetString("coverPages.dir")
	prefix := viper.GetString("coverPages.prefix")
	filename := fmt.Sprintf("%s/%s%d.pdf", dir, prefix, examerID)
	data, err = os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("no uploaded cover page and file not found: %s", filename)
	}
	return data, "coverPages.dir", nil
}

func (p *Plexams) SendCoverPagesMails(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condCoverPagesSent, run); err != nil {
		return err
	}
	teachers, err := p.ExamersWithExamsPlannedByMe(ctx)
	if err != nil {
		return err
	}

	sent := 0
	for _, teacher := range teachers {
		if err := p.SendCoverPageMail(ctx, teacher.ID, run, reporter); err != nil {
			log.Error().Err(err).Int("examerID", teacher.ID).Msg("cannot send cover page mail")
		} else {
			sent++
		}
	}

	if run {
		p.markCondition(ctx, condCoverPagesSent)
	}
	reporter.Println(fmt.Sprintf("sent %d of %d cover-page emails", sent, len(teachers)))
	return nil
}

func (p *Plexams) SendCoverPageMail(ctx context.Context, examerID int, run bool, reporter Reporter) error {
	teacher, err := p.GetTeacher(ctx, examerID)
	if err != nil {
		log.Debug().Err(err).Msg("cannot get teacher by ID")
		return err
	}

	reporter.Step(fmt.Sprintf("cover pages for %s (%d)", teacher.Fullname, examerID))

	pdfData, source, err := p.coverPagePDF(ctx, examerID)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("%s: %v", teacher.Fullname, err))
		return err
	}

	coverMailData := &CoverMailData{
		PlanerName:    p.planer.Name,
		Teacher:       teacher,
		GeneratorName: "Prof. Dr. Edda Eich-Söllner",
	}

	text, html, err := p.mailRenderer().Render("coverPageEmail.md.tmpl", false, coverMailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Deckblätter für Ihre Prüfungen",
		p.semester)

	err = p.sendMail(run,
		[]string{teacher.Email},
		nil,
		subject,
		text,
		html,
		[]*mailAttachment{{
			Filename:    strings.ReplaceAll(fmt.Sprintf("%s_Deckblaetter_Pruefungen_%s.pdf", p.semester, teacher.Fullname), " ", "_"),
			ContentType: "application/pdf",
			Content:     pdfData,
		}},
		false,
	)
	if err != nil {
		reporter.StopProgressFail(fmt.Sprintf("error while sending email to %s: %v", teacher.Fullname, err))
		return err
	}

	reporter.StopProgress(fmt.Sprintf("✓ sent to %s %s [%s]", teacher.Fullname, p.recipientInfo(run, teacher.Email), source))
	return nil
}
