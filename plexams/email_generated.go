package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/jordan-wright/email"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) SendGeneratedExamMail(ctx context.Context, ancode int, run bool) error {
	generatedExam, err := p.GeneratedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get generated exam")
		return err
	}

	if generatedExam.Constraints != nil && generatedExam.Constraints.NotPlannedByMe {
		return fmt.Errorf("not planned by me")
	}

	teacher, err := p.GetTeacher(ctx, generatedExam.ZpaExam.MainExamerID)
	if err != nil {
		log.Error().Err(err).Int("ancode", generatedExam.Ancode).Msg("cannot get teacher")
		return err
	}

	teachersMap := make(map[int]*model.Teacher)
	teachersMap[teacher.ID] = teacher

	err = p.sendGeneratedExamMail(generatedExam, teachersMap, run)
	if err != nil {
		log.Error().Err(err).Int("ancode", generatedExam.Ancode).Msg("cannot send email")
	}
	return nil
}

func (p *Plexams) SendGeneratedExamMails(ctx context.Context, run bool) error {
	generatedExams, err := p.GeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
		return err
	}

	notFromZPA := false
	teachers, err := p.GetTeachers(ctx, &notFromZPA)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
		return err
	}

	teachersMap := make(map[int]*model.Teacher)
	for _, teacher := range teachers {
		teachersMap[teacher.ID] = teacher
	}

	for _, exam := range generatedExams {
		err = p.sendGeneratedExamMail(exam, teachersMap, run)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot send email")
		}
	}

	return nil
}

func (p *Plexams) sendGeneratedExamMail(exam *model.GeneratedExam, teachersMap map[int]*model.Teacher, run bool) error {
	cfg := yacspin.Config{
		Frequency: 100 * time.Millisecond,
		CharSet:   yacspin.CharSets[69],
		Suffix: aurora.Sprintf(aurora.Cyan(" sending email about exam %d. %s (%s)"),
			aurora.Yellow(exam.ZpaExam.AnCode),
			aurora.Magenta(exam.ZpaExam.Module),
			aurora.Magenta(exam.ZpaExam.MainExamer),
		),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "not planned by me",
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

	if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		return nil
	}
	teacher, ok := teachersMap[exam.ZpaExam.MainExamerID]
	if !ok {
		log.Debug().Int("ancode", exam.Ancode).Str("module", exam.ZpaExam.Module).Str("teacher", exam.ZpaExam.MainExamer).
			Msg("no info about teacher in zpa")
		return fmt.Errorf("no info about teacher in zpa")
	}

	var to string
	if run {
		to = teacher.Email
	} else {
		to = "galority@gmail.com"
	}

	hasStudentRegs := false

	for _, primussExam := range exam.PrimussExams {
		hasStudentRegs = hasStudentRegs || len(primussExam.StudentRegs) > 0
	}

	err = p.sendGeneratedExamMailToTeacher(to, &GeneratedExamMailData{
		FromFK07Date:   p.semesterConfig.FromFk07.Format("02.01.2006"),
		ToDate:         p.semesterConfig.Days[len(p.semesterConfig.Days)-1].Date.Format("02.01.2006"),
		Exam:           exam,
		Teacher:        teacher,
		PlanerName:     p.planer.Name,
		HasStudentRegs: hasStudentRegs,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot send email")
		return err
	}
	err = spinner.Stop()
	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}
	return nil
}

type GeneratedExamMailData struct {
	FromFK07Date   string
	ToDate         string
	Exam           *model.GeneratedExam
	Teacher        *model.Teacher
	PlanerName     string
	HasStudentRegs bool
}

func (p *Plexams) sendGeneratedExamMailToTeacher(to string, generatedExamMailData *GeneratedExamMailData) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/generatedExamEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, generatedExamMailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/generatedExamEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, generatedExamMailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Vorliegende Anmeldedaten für Ihre Prüfung %s",
		p.semester, generatedExamMailData.Exam.ZpaExam.Module)
	if !generatedExamMailData.HasStudentRegs {
		subject = fmt.Sprintf("[Prüfungsplanung %s] Keine Anmeldungen für Ihre Prüfung %s",
			p.semester, generatedExamMailData.Exam.ZpaExam.Module)
	}

	attachments := make([]*email.Attachment, 0, 1)

	if generatedExamMailData.HasStudentRegs {
		attachment := &email.Attachment{
			Filename:    fmt.Sprintf("Anmeldungen-%d.csv", generatedExamMailData.Exam.Ancode),
			ContentType: "text/csv; charset=\"utf-8\"",
			Header:      map[string][]string{},
			Content:     []byte("Mtknr;Name;Studiengang;Gruppe\n"),
			HTMLRelated: false,
		}

		for _, primussExam := range generatedExamMailData.Exam.PrimussExams {
			for _, studentReg := range primussExam.StudentRegs {
				attachment.Content = append(attachment.Content,
					[]byte(fmt.Sprintf("\"%s\";%s;%s;%s\n",
						studentReg.Mtknr,
						studentReg.Name,
						studentReg.Program,
						studentReg.Group,
					))...)
			}
		}

		attachments = append(attachments, attachment)

		tmpl, err = template.New("generatedExamMarkdown.tmpl").Funcs(template.FuncMap{
			"add": func(a, b int) int {
				return a + b
			},
		}).ParseFS(emailTemplates, "tmpl/generatedExamMarkdown.tmpl")
		if err != nil {
			return err
		}
		bufMD := new(bytes.Buffer)
		err = tmpl.Execute(bufMD, generatedExamMailData)
		if err != nil {
			return err
		}

		attachment = &email.Attachment{
			Filename:    fmt.Sprintf("Anmeldungen-%d.md", generatedExamMailData.Exam.Ancode),
			ContentType: "text/plain; charset=\"utf-8\"",
			Header:      map[string][]string{},
			Content:     bufMD.Bytes(),
			HTMLRelated: false,
		}
		attachments = append(attachments, attachment)

	}

	return p.sendMail([]string{to},
		nil,
		subject,
		bufText.Bytes(),
		bufHTML.Bytes(),
		attachments,
		false,
	)
}
