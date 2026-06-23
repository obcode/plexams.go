package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/jordan-wright/email"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) SendEmailPublishedExams(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condExamPlanPublished, run); err != nil {
		return err
	}
	reporter.Step("sending email announcing published exams")

	feedbackDate := time.Now().Add(7 * 24 * time.Hour).Format("02.01.06")

	contraintsEmailData := &ConstraintsEmail{
		FromDate:     p.semesterConfig.From.Format("02.01.06"),
		FromFK07Date: p.semesterConfig.FromFk07.Format("02.01.06"),
		UntilDate:    p.semesterConfig.Until.Format("02.01.06"),
		PlanerName:   p.planer.Name,
		FeedbackDate: feedbackDate,
	}

	tmpl, err := template.New("publishedEmailExams.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/publishedEmailExams.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, contraintsEmailData)
	if err != nil {
		return err
	}

	bufHTML, err := p.renderMailHTML("tmpl/publishedEmailExamsHTML.tmpl", true, contraintsEmailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Prüfungsplan veröffentlicht",
		p.semester)

	realRecipients := []string{p.semesterConfig.Emails.Profs, p.semesterConfig.Emails.Lbas, p.semesterConfig.Emails.LbasLastSemester, p.semesterConfig.Emails.Fs}
	realRecipients = append(realRecipients, p.semesterConfig.Emails.AdditionalExamer...)

	if err := p.sendMail(run, realRecipients, nil, subject, bufText.Bytes(), bufHTML, nil, true); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condExamPlanPublished)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, realRecipients...)))
	return nil
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
	Teacher             *model.Teacher
}

// invigilationImagePNG returns the personal invigilation-plan PNG for an
// invigilator: first from the uploaded attachment store (kind invigilation-image,
// key = invigilator id), then falling back to invigilationStats.dir so the CLI
// works without an upload. source is a short label for logging/streaming.
func (p *Plexams) invigilationImagePNG(ctx context.Context, invigilatorID int) (data []byte, source string, err error) {
	att, err := p.GetEmailAttachment(ctx, AttachmentKindInvigilationImage, strconv.Itoa(invigilatorID))
	if err != nil {
		log.Error().Err(err).Int("invigilatorID", invigilatorID).Msg("cannot read invigilation image from store")
	}
	if att != nil && len(att.Data) > 0 {
		return att.Data, "upload", nil
	}

	dir := viper.GetString("invigilationStats.dir")
	prefix := viper.GetString("invigilationStats.prefix")
	filename := fmt.Sprintf("%s/%s%d.png", dir, prefix, invigilatorID)
	data, err = os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("no uploaded invigilation image and file not found: %s", filename)
	}
	return data, "invigilationStats.dir", nil
}

// invigilationImageKeys returns the invigilator ids that have a personal plan
// PNG available, as the union of the upload store (kind invigilation-image) and
// the files in invigilationStats.dir, sorted ascending.
func (p *Plexams) invigilationImageKeys(ctx context.Context) ([]int, error) {
	keys := set.NewSet[int]()

	infos, err := p.EmailAttachmentInfos(ctx, AttachmentKindInvigilationImage)
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if id, err := strconv.Atoi(info.Key); err == nil {
			keys.Add(id)
		}
	}

	if dir := viper.GetString("invigilationStats.dir"); dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Debug().Err(err).Str("dir", dir).Msg("cannot read invigilationStats.dir")
		} else {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if key := keyFromFilename(e.Name()); key != "" {
					if id, err := strconv.Atoi(key); err == nil {
						keys.Add(id)
					}
				}
			}
		}
	}

	ids := keys.ToSlice()
	sort.Ints(ids)
	return ids, nil
}

// SendEmailPublishedInvigilations sends one individual email per invigilator who
// has a personal plan PNG (upload store or invigilationStats.dir), attaching it.
// Invigilators that have an assignment but no calendar are reported and skipped.
func (p *Plexams) SendEmailPublishedInvigilations(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condInvigilationPlanPublished, run); err != nil {
		return err
	}
	reporter.Step("preparing published-invigilation emails")

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

	stats := InvigilationsEmail{
		PlanerName:          p.planer.Name,
		NoOfInvigilators:    invigilationTodos.InvigilatorCount,
		InvigilationInRooms: invigilationTodos.SumExamRooms,
		ReserveInvigilation: invigilationTodos.SumReserve,
		OtherContributions:  invigilationTodos.SumOtherContributions,
		TodoPerInvigilator:  invigilationTodos.TodoPerInvigilatorOvertimeCutted,
		MaxDeviation:        maxDeviation,
		MinDeviation:        -minDeviation,
	}

	textTmpl, err := template.New("publishedInvigilationPersonalEmail.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/publishedInvigilationPersonalEmail.tmpl")
	if err != nil {
		return err
	}
	htmlTmpl, err := template.New("emailBaseHTML.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/emailBaseHTML.tmpl", "tmpl/jiraOnHTML.tmpl", "tmpl/publishedInvigilationPersonalEmailHTML.tmpl")
	if err != nil {
		return err
	}

	// recipients: everyone with a personal plan PNG (upload store or dir).
	ids, err := p.invigilationImageKeys(ctx)
	if err != nil {
		return err
	}
	withCalendar := set.NewSet[int]()
	for _, id := range ids {
		withCalendar.Add(id)
	}

	// Safety net: warn about invigilators that have an assignment but no
	// uploaded/available calendar, so missing uploads are noticed.
	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		return err
	}
	assigned := set.NewSet[int]()
	for _, inv := range invigilations {
		assigned.Add(inv.InvigilatorID)
	}
	missing := assigned.Difference(withCalendar).ToSlice()
	sort.Ints(missing)
	for _, id := range missing {
		if teacher, err := p.GetTeacher(ctx, id); err == nil {
			reporter.Warnf("no calendar for %s (%d) — has invigilations but no PNG, not mailed", teacher.Fullname, id)
		} else {
			reporter.Warnf("no calendar for invigilator %d — has invigilations but no PNG, not mailed", id)
		}
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Ihr Aufsichtenplan", p.semester)

	sent := 0
	for _, id := range ids {
		teacher, err := p.GetTeacher(ctx, id)
		if err != nil {
			log.Error().Err(err).Int("invigilatorID", id).Msg("cannot get teacher")
			reporter.Warnf("cannot get teacher %d: %v", id, err)
			continue
		}

		reporter.Step(fmt.Sprintf("invigilation plan for %s (%d)", teacher.Fullname, id))

		pngData, source, err := p.invigilationImagePNG(ctx, id)
		if err != nil {
			reporter.Warnf("%s: %v", teacher.Fullname, err)
			continue
		}

		data := stats
		data.Teacher = teacher

		bufText := new(bytes.Buffer)
		if err := textTmpl.Execute(bufText, data); err != nil {
			reporter.Warnf("%s: cannot render text: %v", teacher.Fullname, err)
			continue
		}
		bufHTML := new(bytes.Buffer)
		if err := htmlTmpl.Execute(bufHTML, data); err != nil {
			reporter.Warnf("%s: cannot render html: %v", teacher.Fullname, err)
			continue
		}

		baseName := strings.ReplaceAll(fmt.Sprintf("%s_Aufsichtenplan_%s", p.semester, teacher.Fullname), " ", "_")
		attachments := []*email.Attachment{{
			Filename:    baseName + ".png",
			ContentType: "image/png",
			Header:      map[string][]string{},
			Content:     pngData,
			HTMLRelated: false,
		}}

		// Attach an ICS with the same appointments (own exams, invigilations,
		// reserves). A failure here must not stop the mail.
		if icsData, err := p.InvigilatorICS(ctx, id); err != nil {
			reporter.Warnf("%s: cannot build ICS: %v", teacher.Fullname, err)
		} else {
			attachments = append(attachments, &email.Attachment{
				Filename:    baseName + ".ics",
				ContentType: "text/calendar; charset=utf-8",
				Header:      map[string][]string{},
				Content:     icsData,
				HTMLRelated: false,
			})
		}

		err = p.sendMail(run,
			[]string{teacher.Email},
			nil,
			subject,
			bufText.Bytes(),
			bufHTML.Bytes(),
			attachments,
			true,
		)
		if err != nil {
			reporter.Warnf("error while sending email to %s: %v", teacher.Fullname, err)
			continue
		}

		reporter.Printf("  ✓ sent to %s %s [%s]", teacher.Fullname, p.recipientInfo(run, teacher.Email), source)
		sent++
	}

	if run {
		p.markCondition(ctx, condInvigilationPlanPublished)
	}
	reporter.StopProgress(fmt.Sprintf("sent %d of %d invigilation-plan emails", sent, len(ids)))
	return nil
}
