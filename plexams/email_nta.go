package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	// TODO: Ersetzen durch github.com/wneessen/go-mail

	set "github.com/deckarep/golang-set/v2"
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

type NTAEmail struct {
	NTA        *model.Student
	Exams      []*model.PlannedExam
	PlanerName string
}

func (p *Plexams) SendHandicapsMailsNTARoomAlone(ctx context.Context, mtknr string, run bool, reporter Reporter) error {
	// "all" is the bulk send and may go out only once; a single student (specific
	// mtknr) can always be (re-)sent.
	if mtknr == "all" {
		if err := p.emailSendAllowed(ctx, condNTARoomAloneSent, run); err != nil {
			return err
		}
	}
	reporter.Step("sending room-alone emails to NTAs")
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}
	sent := 0
	for _, nta := range ntas {
		if mtknr != "all" && mtknr != nta.Mtknr {
			log.Debug().Str("mtknr", mtknr).Msg("student with mtknr not found")
			continue
		}
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

		to := []string{}
		cc := []string{}
		if nta.Nta.Email != nil {
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

		err = p.SendHandicapsMailToStudentRoomAlone(ctx, run, to, cc, &NTAEmail{
			NTA:        nta,
			Exams:      exams,
			PlanerName: p.planer.Name,
		})
		if err != nil {
			return err
		}
		reporter.Printf("  ✓ %s %s", nta.Name, p.recipientInfo(run, to...))
		sent++
	}
	if run && mtknr == "all" {
		p.markCondition(ctx, condNTARoomAloneSent)
	}
	reporter.StopProgress(fmt.Sprintf("sent %d room-alone emails", sent))
	return nil
}

func (p *Plexams) SendHandicapsMailToStudentRoomAlone(ctx context.Context, run bool, to []string, cc []string, handicapsEmail *NTAEmail) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/handicapEmailRoomAlone.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/handicapEmailRoomAloneHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(run,
		to,
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
	// Waiver is set when the student deliberately gave up their room-alone right
	// for this exam (the stored reason); empty otherwise.
	Waiver string
}

type NTAEmailWithRooms struct {
	NTA           *model.Student
	ExamsWithRoom []NTAEmailExamAndRoom
	PlanerName    string
}

func (p *Plexams) SendHandicapsMailsNTAPlanned(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condNTAPlannedSent, run); err != nil {
		return err
	}
	reporter.Step("sending room-info emails to NTAs")
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	waiverReasons, err := p.ntaRoomAloneWaiverReasons(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get nta room-alone waivers")
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

		if len(exams) == 0 {
			log.Info().Str("mtknr", nta.Mtknr).Str("name", nta.Name).Msg("no exams planned by me")
			continue
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
				Waiver:      waiverReasons[ntaExamKey{nta.Mtknr, exam.Ancode}],
			})
		}
		cc = ccSet.ToSlice()

		err = p.SendHandicapsMailToStudentPlanned(ctx, run, to, cc, &NTAEmailWithRooms{
			NTA:           nta,
			ExamsWithRoom: examsWithRoom,
			PlanerName:    p.planer.Name,
		})
		if err != nil {
			return err
		}
		reporter.Printf("  ✓ %s %s", nta.Name, p.recipientInfo(run, to...))
	}
	if run {
		p.markCondition(ctx, condNTAPlannedSent)
	}
	reporter.StopProgress("room-info emails to NTAs sent")
	return nil
}

func (p *Plexams) SendHandicapsMailToStudentPlanned(ctx context.Context, run bool, to []string, cc []string, handicapsEmail *NTAEmailWithRooms) error {
	log.Debug().Interface("to", to).Msg("sending email")

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/handicapEmailPlanned.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, handicapsEmail)
	if err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/handicapEmailPlannedHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, handicapsEmail)
	if err != nil {
		return err
	}

	return p.sendMail(run,
		to,
		cc,
		fmt.Sprintf("[Prüfungsplanung %s] Räume für Ihre Prüfung(en)", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		true,
	)
}

type NewNTA struct {
	Student    *model.Student
	Exams      []*model.PlannedExam
	PlanerName string
}

func (p *Plexams) SendMailNewNTA(ctx context.Context, mtknr string, run bool, reporter Reporter) error {
	reporter.Step(fmt.Sprintf("informing examers about new NTA %s", mtknr))
	student, err := p.StudentByMtknr(ctx, mtknr)
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).Msg("cannot get nta")
		return err
	}
	if student.Nta == nil {
		log.Error().Str("mtknr", mtknr).Msg("student is not an nta")
		return fmt.Errorf("student is not an nta")
	}
	examerIDsSet := set.NewSet[int]()
	exams := make([]*model.PlannedExam, 0, len(student.Regs))
	for _, ancode := range student.Regs {
		exam, err := p.PlannedExam(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam")
			return err
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		examerIDsSet.Add(exam.ZpaExam.MainExamerID)
		exams = append(exams, exam)
	}
	to := make([]string, 0, examerIDsSet.Cardinality())
	for _, examerID := range examerIDsSet.ToSlice() {
		examer, err := p.GetTeacher(ctx, examerID)
		if err != nil {
			log.Error().Err(err).Int("teacherID", examerID).Msg("cannot get examer")
			return err
		}
		to = append(to, examer.Email)
	}
	log.Debug().Interface("to", to).Msg("sending email to examers about new nta")

	newNTA := &NewNTA{
		Student:    student,
		Exams:      exams,
		PlanerName: p.planer.Name,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/newNTAEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	err = tmpl.Execute(bufText, newNTA)
	if err != nil {
		return err
	}
	tmpl, err = template.ParseFS(emailTemplates, "tmpl/newNTAEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	err = tmpl.Execute(bufHTML, newNTA)
	if err != nil {
		return err
	}

	if err := p.sendMail(run,
		to,
		nil,
		fmt.Sprintf("[Prüfungsplanung %s] Neuer NTA", p.semester),
		bufText.Bytes(),
		bufHTML.Bytes(),
		nil,
		true,
	); err != nil {
		return err
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, to...)))
	return nil
}
