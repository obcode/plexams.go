package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"
	txttmpl "text/template"
	"time"

	set "github.com/deckarep/golang-set/v2"
)

// lbaPerson is a person shown in the LBA-BA email, with their email so the
// LBA-BA can contact them directly.
type lbaPerson struct {
	Name  string
	Email string
}

// lbaProgram is a study program with its number of registrations.
type lbaProgram struct {
	Name  string
	Count int
}

// lbaRepeaterExam is one repeat exam of a non-prof (LBA / Prof HC / external)
// that I planned, reduced to what the Lehrbeauftragten-Beauftragte:r needs: the
// module, the examer and the invigilators (each with email), when it is, and the
// programs with registrations.
type lbaRepeaterExam struct {
	Module       string
	Examer       lbaPerson
	Date         string
	Time         string
	start        time.Time
	Invigilators []lbaPerson  // unique, in room order
	Programs     []lbaProgram // programs with registrations (count > 0)
}

// LbaRepeaterEmail is the data for the LBA-BA overview email.
type LbaRepeaterEmail struct {
	SemesterName string
	PlanerName   string
	Exams        []*lbaRepeaterExam
}

// buildLbaRepeaterExams collects the repeat exams of LBAs that I planned, with
// their time and invigilators, ordered chronologically.
func (p *Plexams) buildLbaRepeaterExams(ctx context.Context) ([]*lbaRepeaterExam, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*lbaRepeaterExam, 0)
	for _, exam := range plannedExams {
		if exam.ZpaExam == nil || !exam.ZpaExam.IsRepeaterExam {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		// LBAs, Prof HC and externals (anyone who is not a regular prof) are the
		// people the LBA-BA reminds. The teachers' IsLBA flag is unreliable here, so
		// "not a regular prof" is the robust criterion.
		mainExamer, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
		if err != nil || mainExamer == nil || mainExamer.IsProf {
			continue
		}

		date, timeStr := "noch nicht geplant", ""
		var start time.Time
		if exam.PlanEntry != nil {
			start = p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			date = fmt.Sprintf("%s, %s", weekdayShortDE[int(start.Weekday())], start.Format("02.01.2006"))
			timeStr = start.Format("15:04")
		}

		invigilators := make([]lbaPerson, 0)
		if exam.PlanEntry != nil {
			seen := set.NewSet[int]()
			for _, room := range exam.PlannedRooms {
				if room.RoomName == "No Room" || room.RoomName == "ONLINE" {
					continue
				}
				invigilator, err := p.GetInvigilatorForRoom(ctx, room.RoomName, exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
				if err != nil || invigilator == nil || seen.Contains(invigilator.ID) {
					continue
				}
				seen.Add(invigilator.ID)
				invigilators = append(invigilators, lbaPerson{Name: invigilator.Shortname, Email: invigilator.Email})
			}
		}

		programs := make([]lbaProgram, 0)
		for _, pe := range exam.PrimussExams {
			if pe == nil || pe.Exam == nil || len(pe.StudentRegs) == 0 {
				continue
			}
			programs = append(programs, lbaProgram{Name: pe.Exam.Program, Count: len(pe.StudentRegs)})
		}
		sort.Slice(programs, func(i, j int) bool { return programs[i].Name < programs[j].Name })

		result = append(result, &lbaRepeaterExam{
			Module:       exam.ZpaExam.Module,
			Examer:       lbaPerson{Name: exam.ZpaExam.MainExamer, Email: mainExamer.Email},
			Date:         date,
			Time:         timeStr,
			start:        start,
			Invigilators: invigilators,
			Programs:     programs,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].start.Before(result[j].start)
	})

	return result, nil
}

// SendEmailLbaRepeaters sends the Lehrbeauftragten-Beauftragte:r (emails.lbaba)
// an overview of all repeat exams of LBAs that I planned — only dates and
// invigilations — so they know whom to remind. Answerable by email (no JIRA).
// Send-once (condLbaRepeatersSent).
func (p *Plexams) SendEmailLbaRepeaters(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condLbaRepeatersSent, run); err != nil {
		return err
	}
	reporter.Step("collecting LBA repeat exams")

	exams, err := p.buildLbaRepeaterExams(ctx)
	if err != nil {
		return err
	}
	if len(exams) == 0 {
		reporter.StopProgress("no LBA repeat exams planned by me, nothing to send")
		return nil
	}

	data := &LbaRepeaterEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Exams:        exams,
	}

	// the affected invigilators go into Cc
	ccSet := make(map[string]bool)
	for _, exam := range exams {
		for _, inv := range exam.Invigilators {
			if inv.Email != "" {
				ccSet[inv.Email] = true
			}
		}
	}
	cc := make([]string, 0, len(ccSet))
	for e := range ccSet {
		cc = append(cc, e)
	}
	sort.Strings(cc)

	textTmpl, err := txttmpl.ParseFS(emailTemplates, "tmpl/lbaRepeaterEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	if err := textTmpl.Execute(bufText, data); err != nil {
		return err
	}

	htmlTmpl, err := template.New("emailBaseHTML.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, "tmpl/emailBaseHTML.tmpl", "tmpl/lbaRepeaterEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	if err := htmlTmpl.Execute(bufHTML, data); err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Wiederholungsprüfungen von Lehrbeauftragten", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Lbaba}, cc, subject, bufText.Bytes(), bufHTML.Bytes(), nil, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condLbaRepeatersSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s (%d exams)", p.recipientInfo(run, p.semesterConfig.Emails.Lbaba), len(exams)))
	return nil
}
