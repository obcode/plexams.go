package email

import (
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// fk07 is the faculty marker (Teacher.FK) of FK07 examers.
const fk07 = "FK07"

// BuildExamPlanningRecipients computes the recipients of the consolidated exam-planning
// info email from already-fetched inputs: one entry per examer with at least one exam I
// plan (in withConstraints and not NotPlannedByMe, any faculty) listing those exams, plus
// the active FK07 examers I plan nothing for. Examers of other faculties without a planned
// exam are excluded. No slot/date is included.
//
// teachers is the local teacher master data (for names/FK/email); allExams is the current
// ZPA exam set (to know which examers have any exam at all this semester).
func BuildExamPlanningRecipients(
	withConstraints []*model.ZPAExamWithConstraints,
	teachers []*model.Teacher,
	allExams []*model.ZPAExam,
) []*model.ExamPlanningMailRecipient {
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

	teacherByID := make(map[int]*model.Teacher, len(teachers))
	for _, t := range teachers {
		teacherByID[t.ID] = t
	}

	// which examers have at least one exam in the current ZPA data at all
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

	return recipients
}
