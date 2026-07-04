package email

import (
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
)

// LbaPerson is a person shown in the LBA-BA email, with their email so the LBA-BA can
// contact them directly.
type LbaPerson struct {
	Name  string
	Email string
}

// LbaProgram is a study program with its number of registrations.
type LbaProgram struct {
	Name  string
	Count int
}

// LbaRepeaterExam is one repeat exam of a non-prof (LBA / Prof HC / external) that I
// planned, reduced to what the Lehrbeauftragten-Beauftragte:r needs: the module, the
// examer and the invigilators (each with email), when it is, and the programs with
// registrations.
type LbaRepeaterExam struct {
	Module       string
	Examer       LbaPerson
	Date         string
	Time         string
	start        time.Time
	Invigilators []LbaPerson  // unique, in room order
	Programs     []LbaProgram // programs with registrations (count > 0)
}

// LbaRepeaterEmail is the data for the LBA-BA overview email.
type LbaRepeaterEmail struct {
	SemesterName string
	PlanerName   string
	Exams        []*LbaRepeaterExam
}

// BuildLbaRepeaterExams collects the repeat exams of LBAs (non-profs) that I planned, with
// their time and invigilators, ordered chronologically. Pure over the already-fetched
// exams: examer resolves an examer ID to its teacher (nil ⇒ skip the exam, e.g. lookup
// error); only non-profs are kept. invigilatorForRoom resolves a room in a slot to its
// invigilator (nil ⇒ skip that room); slotTime resolves a plan entry to its start.
func BuildLbaRepeaterExams(
	plannedExams []*model.PlannedExam,
	slotTime func(day, slot int) time.Time,
	examer func(id int) *model.Teacher,
	invigilatorForRoom func(room string, day, slot int) *model.Teacher,
) []*LbaRepeaterExam {
	result := make([]*LbaRepeaterExam, 0)
	for _, exam := range plannedExams {
		if exam.ZpaExam == nil || !exam.ZpaExam.IsRepeaterExam {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		// LBAs, Prof HC and externals (anyone who is not a regular prof) are the people the
		// LBA-BA reminds. The teachers' IsLBA flag is unreliable here, so "not a regular
		// prof" is the robust criterion.
		mainExamer := examer(exam.ZpaExam.MainExamerID)
		if mainExamer == nil || mainExamer.IsProf {
			continue
		}

		date, timeStr := "noch nicht geplant", ""
		var start time.Time
		if exam.PlanEntry != nil {
			start = slotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			date = DateDE(start)
			timeStr = TimeDE(start)
		}

		invigilators := make([]LbaPerson, 0)
		if exam.PlanEntry != nil {
			seen := set.NewSet[int]()
			for _, room := range exam.PlannedRooms {
				if room.RoomName == "ONLINE" {
					continue
				}
				invigilator := invigilatorForRoom(room.RoomName, exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
				if invigilator == nil || seen.Contains(invigilator.ID) {
					continue
				}
				seen.Add(invigilator.ID)
				invigilators = append(invigilators, LbaPerson{Name: invigilator.Shortname, Email: invigilator.Email})
			}
		}

		programs := make([]LbaProgram, 0)
		for _, pe := range exam.PrimussExams {
			if pe == nil || pe.Exam == nil || len(pe.StudentRegs) == 0 {
				continue
			}
			programs = append(programs, LbaProgram{Name: pe.Exam.Program, Count: len(pe.StudentRegs)})
		}
		sort.Slice(programs, func(i, j int) bool { return programs[i].Name < programs[j].Name })

		result = append(result, &LbaRepeaterExam{
			Module:       exam.ZpaExam.Module,
			Examer:       LbaPerson{Name: exam.ZpaExam.MainExamer, Email: mainExamer.Email},
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

	return result
}
