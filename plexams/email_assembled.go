package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	txttmpl "text/template"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) SendAssembledExamMail(ctx context.Context, ancode int, updated, run bool, reporter Reporter) error {
	assembledExam, err := p.AssembledExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get assembled exam")
		return err
	}

	if assembledExam.Constraints != nil && assembledExam.Constraints.NotPlannedByMe {
		return fmt.Errorf("not planned by me")
	}

	teacher, err := p.GetTeacher(ctx, assembledExam.ZpaExam.MainExamerID)
	if err != nil {
		log.Error().Err(err).Int("ancode", assembledExam.Ancode).Msg("cannot get teacher")
		return err
	}

	teachersMap := make(map[int]*model.Teacher)
	teachersMap[teacher.ID] = teacher

	err = p.sendAssembledExamMail(assembledExam, teachersMap, updated, run, reporter)
	if err != nil {
		log.Error().Err(err).Int("ancode", assembledExam.Ancode).Msg("cannot send email")
	}
	return nil
}

func (p *Plexams) SendAssembledExamMails(ctx context.Context, emailAddresses, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condPrimussDataAllSent, run); err != nil {
		return err
	}
	assembledExams, err := p.AssembledExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
		return err
	}

	notFromZPA := false
	teachers, err := p.GetTeachers(ctx, &notFromZPA)
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
		return err
	}

	teachersMap := make(map[int]*model.Teacher)
	for _, teacher := range teachers {
		teachersMap[teacher.ID] = teacher
	}

	for _, exam := range assembledExams {
		err = p.sendAssembledExamMail(exam, teachersMap, false, run, reporter)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot send email")
		}
	}
	if run {
		p.markCondition(ctx, condPrimussDataAllSent)
	}
	reporter.StopProgress(fmt.Sprintf("sent %d primuss-data emails", len(assembledExams)))
	return nil
}

func (p *Plexams) sendAssembledExamMail(exam *model.AssembledExam, teachersMap map[int]*model.Teacher, updated, run bool, reporter Reporter) error {
	reporter.Step(aurora.Sprintf(aurora.Cyan("sending email about exam %d. %s (%s)"),
		aurora.Yellow(exam.ZpaExam.AnCode),
		aurora.Magenta(exam.ZpaExam.Module),
		aurora.Magenta(exam.ZpaExam.MainExamer),
	))

	if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
		reporter.Warnf("exam %d not planned by me, skipped", exam.ZpaExam.AnCode)
		return nil
	}
	teacher, ok := teachersMap[exam.ZpaExam.MainExamerID]
	if !ok {
		log.Debug().Int("ancode", exam.Ancode).Str("module", exam.ZpaExam.Module).Str("teacher", exam.ZpaExam.MainExamer).
			Msg("no info about teacher in zpa")
		return fmt.Errorf("no info about teacher in zpa")
	}

	to := teacher.Email

	hasStudentRegs := false

	for _, primussExam := range exam.PrimussExams {
		hasStudentRegs = hasStudentRegs || len(primussExam.StudentRegs) > 0
	}

	log.Debug().Int("ancode", exam.Ancode).Bool("hasStudentRegs", hasStudentRegs).Msg("found student regs for exam")

	if err := p.sendAssembledExamMailToTeacher(run, to, &AssembledExamMailData{
		FromDate:       p.semesterConfig.From.Format("02.01.2006"),
		ToDate:         p.semesterConfig.Days[len(p.semesterConfig.Days)-1].Date.Format("02.01.2006"),
		Exam:           exam,
		Teacher:        teacher,
		PlanerName:     p.planer.Name,
		HasStudentRegs: hasStudentRegs,
	}, updated); err != nil {
		log.Error().Err(err).Msg("cannot send email")
		return err
	}
	reporter.Printf("  ✓ %d. %s -> %s", exam.ZpaExam.AnCode, exam.ZpaExam.Module, p.recipientInfo(run, to))
	return nil
}

type AssembledExamMailData struct {
	FromDate       string
	ToDate         string
	Exam           *model.AssembledExam
	Teacher        *model.Teacher
	PlanerName     string
	HasStudentRegs bool
}

func (p *Plexams) sendAssembledExamMailToTeacher(run bool, to string, assembledExamMailData *AssembledExamMailData, updated bool) error {
	log.Debug().Interface("to", to).Msg("sending email")

	text, html, err := p.renderMarkdownEmail("assembledExamEmail.md.tmpl", true, assembledExamMailData)
	if err != nil {
		return err
	}

	attribute := "Vorliegende"
	if updated {
		attribute = "Aktualisierte"
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] %s Anmeldedaten für Ihre Prüfung %s",
		p.semester, attribute, assembledExamMailData.Exam.ZpaExam.Module)
	if !assembledExamMailData.HasStudentRegs {
		subject = fmt.Sprintf("[Prüfungsplanung %s] Keine Anmeldungen für Ihre Prüfung %s",
			p.semester, assembledExamMailData.Exam.ZpaExam.Module)
	}

	var attachments []*mailAttachment

	if assembledExamMailData.HasStudentRegs {
		attachments = make([]*mailAttachment, 0, 1)
		var attachment *mailAttachment

		if assembledExamMailData.HasStudentRegs {
			attachment = &mailAttachment{
				Filename:    fmt.Sprintf("Anmeldungen-%d.csv", assembledExamMailData.Exam.Ancode),
				ContentType: "text/csv; charset=\"utf-8\"",
				Content:     []byte("Mtknr;Name;Gender;E-Mail;Studiengang;Gruppe\n"),
			}

			for _, primussExam := range assembledExamMailData.Exam.PrimussExams {
				for _, studentReg := range primussExam.StudentRegs {
					// force Excel/Numbers to treat the field as text with leading zeros:
					// write the Mtknr as an Excel formula: ="000123"
					gender := ""
					email := ""

					if studentReg.ZpaStudent != nil {
						gender = studentReg.ZpaStudent.Gender
						email = studentReg.ZpaStudent.Email
					}

					attachment.Content = append(attachment.Content,
						[]byte(fmt.Sprintf("=\"%s\";%s;%s;%s;%s;%s\n",
							studentReg.Mtknr,
							studentReg.Name,
							gender,
							email,
							studentReg.Program,
							studentReg.Group,
						))...)
				}
			}

		}
		attachments = append(attachments, attachment)

		txttmpl, err := txttmpl.New("assembledExamMarkdown.tmpl").Funcs(template.FuncMap{
			"add": func(a, b int) int {
				return a + b
			},
		}).ParseFS(emailTemplates, "tmpl/assembledExamMarkdown.tmpl")
		if err != nil {
			return err
		}
		bufMD := new(bytes.Buffer)
		err = txttmpl.Execute(bufMD, assembledExamMailData)
		if err != nil {
			return err
		}

		attachment = &mailAttachment{
			Filename:    fmt.Sprintf("Anmeldungen-%d.md", assembledExamMailData.Exam.Ancode),
			ContentType: "text/plain; charset=\"utf-8\"",
			Content:     bufMD.Bytes(),
		}
		attachments = append(attachments, attachment)
	}

	return p.sendMail(run,
		[]string{to},
		nil,
		subject,
		text,
		html,
		attachments,
		true,
	)
}

type UnpplannedExamMailData struct {
	Exam       *model.PrimussExam
	PlanerName string
}

func (p *Plexams) SendUnplannedExamMail(ctx context.Context, program string, ancode int, emailAddress string, run bool, reporter Reporter) error {
	reporter.Step(fmt.Sprintf("sending primuss data for unplanned exam %s/%d", program, ancode))
	exam, err := p.dbClient.GetPrimussExam(ctx, program, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Str("program", program).Msg("cannot get primuss exam")
		return err
	}
	studentRegs, err := p.GetEnhancedStudentRegs(ctx, program, ancode)
	if err != nil {
		log.Debug().Err(err).Int("ancode", ancode).Str("program", program).Msg("cannot get primuss student registrations")
	}
	subject := fmt.Sprintf("[Prüfungsplanung %s] Anmeldedaten für Ihre Prüfung %s im Studiengang %s",
		p.semester, exam.Module, program)

	if len(studentRegs) > 0 {
		attachments := make([]*mailAttachment, 0, 1)

		attachment := &mailAttachment{
			Filename:    fmt.Sprintf("Anmeldungen-%s-%d.csv", program, ancode),
			ContentType: "text/csv; charset=\"utf-8\"",
			Content:     []byte("Mtknr;Name;Gender;E-Mail;Studiengang;Gruppe\n"),
		}

		for _, studentReg := range studentRegs {
			// force Excel/Numbers to treat the field as text with leading zeros:
			// write the Mtknr as an Excel formula: ="000123"
			gender := ""
			email := ""

			if studentReg.ZpaStudent != nil {
				gender = studentReg.ZpaStudent.Gender
				email = studentReg.ZpaStudent.Email
			}

			attachment.Content = append(attachment.Content,
				[]byte(fmt.Sprintf("=\"%s\";%s;%s;%s;%s;%s\n",
					studentReg.Mtknr,
					studentReg.Name,
					gender,
					email,
					studentReg.Program,
					studentReg.Group,
				))...)
		}

		attachments = append(attachments, attachment)

		unplannedExamData := &UnpplannedExamMailData{
			Exam:       exam,
			PlanerName: p.planer.Name,
		}

		text, html, err := p.renderMarkdownEmail("unplannedExamEmail.md.tmpl", false, unplannedExamData)
		if err != nil {
			return err
		}

		if err := p.sendMail(run,
			[]string{emailAddress},
			nil,
			subject,
			text,
			html,
			attachments,
			false,
		); err != nil {
			return err
		}
		reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, emailAddress)))
		return nil
	}
	reporter.StopProgressFail(fmt.Sprintf("no student registrations found for %s/%d", program, ancode))
	return nil
}
