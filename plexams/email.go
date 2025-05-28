package plexams

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"

	// TODO: Ersetzen durch github.com/wneessen/go-mail

	set "github.com/deckarep/golang-set/v2"
	"github.com/jordan-wright/email"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
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
		nil,
		fmt.Sprintf("[Prüfungsplanung %s] Nachteilausgleich(e) für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		false,
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
		cc := []string{}
		if run && nta.Nta.Email != nil {
			to = []string{*nta.Nta.Email}
			for _, exam := range exams {
				teacher, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
				if err != nil {
					log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get teacher")
					return err
				}
				cc = append(cc, teacher.Email)
			}
		}

		err = p.SendHandicapsMailToStudentRoomAlone(ctx, to, cc, &NTAEmail{
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

func (p *Plexams) SendHandicapsMailToStudentRoomAlone(ctx context.Context, to []string, cc []string, handicapsEmail *NTAEmail) error {
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
		cc,
		fmt.Sprintf("[Prüfungsplanung %s] Eigener Raum für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		false,
	)
}

type NTAEmailExamAndRoom struct {
	Exam        *model.PlannedExam
	Room        *model.PlannedRoom
	Invigilator *model.Teacher
}

type NTAEmailWithRooms struct {
	NTA           *model.Student
	ExamsWithRoom []NTAEmailExamAndRoom
	PlanerName    string
}

func (p *Plexams) SendHandicapsMailsNTAPlanned(ctx context.Context, run bool) error {
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	atLeastOneEmailMissing := false
	for _, nta := range ntas {
		if nta.Nta.Email == nil || *nta.Nta.Email == "" {
			log.Error().Str("mtknr", nta.Mtknr).Str("name", nta.Name).Msg("no email set")
			atLeastOneEmailMissing = true
		}
	}
	if atLeastOneEmailMissing {
		return fmt.Errorf("at least one email missing")
	}

	for _, nta := range ntas {
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

		var to, cc []string

		examsWithRoom := make([]NTAEmailExamAndRoom, 0, len(exams))
		to = []string{*nta.Nta.Email}
		ccSet := set.NewSet[string]()
		for _, exam := range exams {
			teacher, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get teacher")
				return err
			}
			ccSet.Add(teacher.Email)
			// invigilators cc
			room, err := p.PlannedRoomForStudent(ctx, exam.Ancode, nta.Mtknr)
			if room == nil || err != nil {
				log.Error().Int("ancode", exam.Ancode).Str("mtknr", nta.Mtknr).Msg("no room")
				continue
			}
			invigilator, err := p.GetInvigilatorInSlot(ctx, room.RoomName, exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			if err != nil || invigilator == nil {
				log.Error().Err(err).Int("ancode", exam.Ancode).Str("room", room.RoomName).
					Int("slot", exam.PlanEntry.SlotNumber).Int("day", exam.PlanEntry.DayNumber).
					Msg("cannot get invigilator")
				continue
			}
			log.Debug().Str("mtknr", nta.Mtknr).Str("name", nta.Name).Str("room", room.RoomName).Str("invigilator", invigilator.Fullname).
				Msg("found info")
			ccSet.Add(invigilator.Email)
			examsWithRoom = append(examsWithRoom, NTAEmailExamAndRoom{
				Exam:        exam,
				Room:        room,
				Invigilator: invigilator,
			})
		}
		cc = ccSet.ToSlice()

		if !run {
			to = []string{p.planer.Email}
			log.Debug().Interface("cc", cc).Msg("not sending cc")
			cc = []string{}
		}

		err = p.SendHandicapsMailToStudentPlanned(ctx, to, cc, &NTAEmailWithRooms{
			NTA:           nta,
			ExamsWithRoom: examsWithRoom,
			PlanerName:    p.planer.Name,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Plexams) SendHandicapsMailToStudentPlanned(ctx context.Context, to []string, cc []string, handicapsEmail *NTAEmailWithRooms) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFiles("tmpl/handicapEmailPlanned.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFiles("tmpl/handicapEmailPlannedHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(to,
		cc,
		fmt.Sprintf("[Prüfungsplanung %s] Räume für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		true,
	)
}

func (p *Plexams) sendMail(to []string, cc []string, subject string, text []byte, html []byte, attachments []*email.Attachment, noreply bool) error {
	e := &email.Email{
		To:          to,
		Cc:          cc,
		Bcc:         []string{p.planer.Email},
		From:        fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject:     subject,
		Text:        text,
		HTML:        html,
		Headers:     textproto.MIMEHeader{},
		Attachments: attachments,
	}

	if noreply {
		e.ReplyTo = []string{"obraun+noreply@hm.edu"}
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
