package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// fk07 is the faculty marker (Teacher.FK) of FK07 examers.
const fk07 = "FK07"

// ExamPlanningMailRecipients computes the recipients of the consolidated exam-planning
// info email: one entry per examer with at least one exam I plan (toPlan and not
// notPlannedByMe, any faculty) listing those exams, plus the active FK07 examers I plan
// nothing for. Examers of other faculties without a planned exam are excluded. No
// slot/date is included. Used as the pre-step so the planner can select/deselect.
func (p *Plexams) ExamPlanningMailRecipients(ctx context.Context) ([]*model.ExamPlanningMailRecipient, error) {
	withConstraints, err := p.ZpaExamsToPlanWithConstraints(ctx)
	if err != nil {
		return nil, err
	}

	// planned-by-me exams grouped by main examer
	type group struct {
		name  string
		exams []*model.ExamPlanningMailExam
	}
	byExamer := make(map[int]*group)
	for _, ewc := range withConstraints {
		if ewc.Constraints != nil && ewc.Constraints.NotPlannedByMe {
			continue
		}
		ze := ewc.ZpaExam
		g := byExamer[ze.MainExamerID]
		if g == nil {
			g = &group{name: ze.MainExamer}
			byExamer[ze.MainExamerID] = g
		}
		g.exams = append(g.exams, &model.ExamPlanningMailExam{
			Ancode:      ze.AnCode,
			Module:      ze.Module,
			ExamType:    ze.ExamTypeFull,
			Constraints: ewc.Constraints,
		})
	}

	fromZPA := false
	teachers, err := p.GetTeachers(ctx, &fromZPA)
	if err != nil {
		return nil, err
	}
	teacherByID := make(map[int]*model.Teacher, len(teachers))
	for _, t := range teachers {
		teacherByID[t.ID] = t
	}

	// which examers have at least one exam in the current ZPA data at all
	allExams, err := p.GetZPAExams(ctx, &fromZPA)
	if err != nil {
		return nil, err
	}
	hasExam := make(map[int]bool)
	for _, e := range allExams {
		hasExam[e.MainExamerID] = true
	}

	recipients := make([]*model.ExamPlanningMailRecipient, 0)

	// examers with exams I plan (any faculty)
	for id, g := range byExamer {
		teacher := teacherByID[id]
		if teacher == nil {
			// examer not in the teachers master data (e.g. external) — minimal stub so
			// the recipient still shows; the GUI flags the missing email.
			teacher = &model.Teacher{ID: id, Fullname: g.name}
		}
		sort.Slice(g.exams, func(i, j int) bool { return g.exams[i].Ancode < g.exams[j].Ancode })
		recipients = append(recipients, &model.ExamPlanningMailRecipient{
			Teacher:  teacher,
			Category: "withExams",
			Exams:    g.exams,
		})
	}

	// FK07 examers who have ZPA exam(s) this semester but none that I plan
	for _, t := range teachers {
		if t.FK == fk07 && hasExam[t.ID] && byExamer[t.ID] == nil {
			recipients = append(recipients, &model.ExamPlanningMailRecipient{
				Teacher:  t,
				Category: "fk07NoExams",
				Exams:    []*model.ExamPlanningMailExam{},
			})
		}
	}

	// withExams first, then fk07NoExams; each alphabetically by name
	sort.SliceStable(recipients, func(i, j int) bool {
		if recipients[i].Category != recipients[j].Category {
			return recipients[i].Category == "withExams"
		}
		return recipients[i].Teacher.Fullname < recipients[j].Teacher.Fullname
	})

	return recipients, nil
}

// examPlanningInfoMailData is the template data for one recipient.
type examPlanningInfoMailData struct {
	Teacher    *model.Teacher
	Category   string
	FromDate   string
	UntilDate  string
	PlanerName string
	Exams      []*model.ExamPlanningMailExam
}

// SendExamPlanningInfoMails sends the consolidated exam-planning info email to the given
// examers (teacherIDs; empty = all candidates). Per examer it lists the exams I plan with
// their constraints (no slot/date), or — for FK07 examers I plan nothing for — a note
// that I plan none of theirs. run=false is a dry run (only the planner is mailed).
func (p *Plexams) SendExamPlanningInfoMails(ctx context.Context, teacherIDs []int, run bool, reporter Reporter) error {
	recipients, err := p.ExamPlanningMailRecipients(ctx)
	if err != nil {
		return err
	}
	if len(teacherIDs) > 0 {
		want := make(map[int]bool, len(teacherIDs))
		for _, id := range teacherIDs {
			want[id] = true
		}
		filtered := recipients[:0]
		for _, r := range recipients {
			if want[r.Teacher.ID] {
				filtered = append(filtered, r)
			}
		}
		recipients = filtered
	}

	sent, skipped := 0, 0
	for _, r := range recipients {
		reporter.Step(fmt.Sprintf("%s (%s)", r.Teacher.Fullname, r.Category))
		if strings.TrimSpace(r.Teacher.Email) == "" {
			reporter.StopProgressFail(fmt.Sprintf("%s: keine E-Mail-Adresse — übersprungen", r.Teacher.Fullname))
			skipped++
			continue
		}

		data := &examPlanningInfoMailData{
			Teacher:    r.Teacher,
			Category:   r.Category,
			FromDate:   p.semesterConfig.From.Format("02.01.06"),
			UntilDate:  p.semesterConfig.Until.Format("02.01.06"),
			PlanerName: p.planer.Name,
			Exams:      r.Exams,
		}

		text, html, err := p.renderMarkdownEmail("examPlanningInfoEmail.md.tmpl", true, data)
		if err != nil {
			return err
		}
		subjectExams := "Ihre Prüfungen in der Prüfungsplanung der FK07"
		if r.Category != "withExams" {
			subjectExams = "Keine Prüfungen in der Prüfungsplanung der FK07"
		}
		subject := fmt.Sprintf("[Prüfungsplanung %s] %s", p.semester, subjectExams)
		if err := p.sendMail(run, []string{r.Teacher.Email}, nil, subject, text, html, nil, true); err != nil {
			reporter.StopProgressFail(fmt.Sprintf("%s: %v", r.Teacher.Fullname, err))
			log.Error().Err(err).Int("teacherID", r.Teacher.ID).Msg("cannot send exam-planning info mail")
			continue
		}
		sent++
	}

	if run {
		p.markCondition(ctx, condExamPlanningInfoSent)
	}
	// StopProgress (not Println) so the last transient per-recipient line is replaced
	// by the summary instead of lingering.
	reporter.StopProgress(fmt.Sprintf("%d E-Mail(s) verschickt, %d übersprungen", sent, skipped))
	return nil
}

// constraintsText renders a short, human-readable summary of an exam's constraints for
// the email (no slot/date — that is not part of Constraints). Returns "" when there is
// nothing noteworthy.
func constraintsText(c *model.Constraints) string {
	if c == nil {
		return ""
	}
	parts := make([]string, 0)
	if len(c.SameSlot) > 0 {
		ancodes := make([]string, 0, len(c.SameSlot))
		for _, a := range c.SameSlot {
			ancodes = append(ancodes, strconv.Itoa(a))
		}
		parts = append(parts, "gleicher Slot wie "+strings.Join(ancodes, ", "))
	}
	if c.Online {
		parts = append(parts, "online")
	}
	if rc := c.RoomConstraints; rc != nil {
		if rc.Exahm {
			parts = append(parts, "EXaHM")
		}
		if rc.Seb {
			parts = append(parts, "SEB")
		}
		if rc.Lab {
			parts = append(parts, "Labor")
		}
		if rc.PlacesWithSocket {
			parts = append(parts, "Steckdosen")
		}
		if len(rc.AllowedRooms) > 0 {
			parts = append(parts, "nur Räume: "+strings.Join(rc.AllowedRooms, ", "))
		}
		if rc.MaxStudents != nil && *rc.MaxStudents > 0 {
			parts = append(parts, fmt.Sprintf("max. %d Studierende/Raum", *rc.MaxStudents))
		}
	}
	if c.FixedDay != nil {
		parts = append(parts, "fester Tag gewünscht")
	}
	return strings.Join(parts, "; ")
}
