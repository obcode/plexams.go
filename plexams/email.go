package plexams

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"
	"time"

	// TODO: Ersetzen durch github.com/wneessen/go-mail
	"github.com/jordan-wright/email"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

type HandicapsEmail struct {
	MainExamer string
	Exams      []*HandicapExam
	PlanerName string
}

type HandicapExam struct {
	AnCode           int
	Module           string
	TypeExamFull     string
	HandicapStudents []*HandicapStudent
}

type HandicapStudent struct {
	Name         string
	Compensation string
}

func (p *Plexams) SendTestMail() error {
	e := &email.Email{
		To:      []string{p.planer.Email},
		From:    fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject: "Awesome Subject",
		Text:    []byte("Text Body is, of course, supported!"),
		HTML:    []byte("<h1>Fancy HTML is supported, too!</h1>"),
		Headers: textproto.MIMEHeader{},
	}

	return e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})
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
			continue
		}
		teacher, ok := teachersMap[exam.ZpaExam.MainExamerID]
		if !ok {
			log.Debug().Int("ancode", exam.Ancode).Str("module", exam.ZpaExam.Module).Str("teacher", exam.ZpaExam.MainExamer).
				Msg("no info about teacher in zpa")
			continue
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

		err = p.SendGeneratedExamMailToTeacher(ctx, to, &GeneratedExamMailData{
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
	}

	return nil
}

type GeneratedExamMailData struct {
	Exam           *model.GeneratedExam
	Teacher        *model.Teacher
	PlanerName     string
	HasStudentRegs bool
}

func (p *Plexams) SendGeneratedExamMailToTeacher(ctx context.Context, to string, generatedExamMailData *GeneratedExamMailData) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFiles("tmpl/generatedExamEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, generatedExamMailData)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFiles("tmpl/generatedExamEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, generatedExamMailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Vorliegende Planungsdaten für Ihre Prüfung %s",
		p.semester, generatedExamMailData.Exam.ZpaExam.Module)
	if !generatedExamMailData.HasStudentRegs {
		subject = fmt.Sprintf("[Prüfungsplanung %s] Keine Anmeldungen für Ihre Prüfung %s",
			p.semester, generatedExamMailData.Exam.ZpaExam.Module)
	}

	return p.sendMail([]string{to},
		subject,
		bufText.Bytes(),
		bufHTML.Bytes(),
	)
}

// Deprecated: rm me
func (p *Plexams) SendHandicapsMails(ctx context.Context, run bool) error {
	ntasByTeacher, err := p.NtasWithRegsByTeacher(ctx)
	if err != nil {
		return err
	}

	for _, nta := range ntasByTeacher {
		exams := make([]*HandicapExam, 0, len(nta.Exams))
		for _, exam := range nta.Exams {
			handicapStudents := make([]*HandicapStudent, 0, len(exam.Ntas))
			for _, ntaForExam := range exam.Ntas {
				handicapStudents = append(handicapStudents, &HandicapStudent{
					Name:         ntaForExam.Nta.Name,
					Compensation: ntaForExam.Nta.Compensation,
				})
			}

			exams = append(exams, &HandicapExam{
				AnCode:           exam.Exam.AnCode,
				Module:           exam.Exam.Module,
				TypeExamFull:     exam.Exam.ExamTypeFull,
				HandicapStudents: handicapStudents,
			})
		}

		var to []string
		if run {
			to = []string{nta.Teacher.Email}
		} else {
			to = []string{"galority@gmail.com"}
		}

		err = p.SendHandicapsMailToMainExamer(ctx, to, &HandicapsEmail{
			MainExamer: nta.Teacher.Fullname,
			Exams:      exams,
			PlanerName: p.planer.Name,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Deprecated: rm me
func (p *Plexams) SendHandicapsMailToMainExamer(ctx context.Context, to []string, handicapsEmail *HandicapsEmail) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFiles("tmpl/handicapEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFiles("tmpl/handicapEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(to,
		fmt.Sprintf("[Prüfungsplanung %s] Nachteilausgleich(e) für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
	)
}

type NTAEmail struct {
	NTA        *model.Student
	Exams      []*model.PlannedExam
	PlanerName string
}

func (p *Plexams) SendHandicapsMailsNTARoomAlone(ctx context.Context, run bool) error {
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}
	for _, nta := range ntas {
		if !nta.Nta.NeedsRoomAlone {
			continue
		}

		exams := make([]*model.PlannedExam, 0, len(nta.Regs))
		for _, ancode := range nta.Regs {
			exam, err := p.PlannedExam(ctx, ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam")
				return err
			}
			if exam.Constraints == nil || !exam.Constraints.NotPlannedByMe {
				exams = append(exams, exam)
			}
		}

		to := []string{p.planer.Email}
		if run && nta.Nta.Email != nil {
			to = []string{*nta.Nta.Email}
		}

		err = p.SendHandicapsMailToStudent(ctx, to, &NTAEmail{
			NTA:        nta,
			Exams:      exams,
			PlanerName: p.planer.Name,
		})
		if err != nil {
			return err
		}

	}
	return nil
}

func (p *Plexams) SendHandicapsMailToStudent(ctx context.Context, to []string, handicapsEmail *NTAEmail) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFiles("tmpl/handicapEmailRoomAlone.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFiles("tmpl/handicapEmailRoomAloneHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(to,
		fmt.Sprintf("[Prüfungsplanung %s] Eigener Raum für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
	)
}

func (p *Plexams) sendMail(to []string, subject string, text []byte, html []byte) error {
	e := &email.Email{
		To:      to,
		Bcc:     []string{p.planer.Email},
		From:    fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject: subject,
		Text:    text,
		HTML:    html,
		Headers: textproto.MIMEHeader{},
	}

	err := e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})

	if err != nil {
		return err
	}

	return p.Log(context.Background(), fmt.Sprintf("send email to %s", to), string(text))
}
