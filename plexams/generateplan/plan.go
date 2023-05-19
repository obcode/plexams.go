package generateplan

import (
	"math/rand"

	"github.com/obcode/plexams.go/graph/model"
)

type Exam struct {
	examGroup *model.ExamGroup
	day       int
	slot      int
}

func newExam(examGroup *model.ExamGroup) *Exam {
	return &Exam{examGroup: examGroup}
}

func (exam *Exam) randomSlot() {
	possibleSlots := exam.examGroup.ExamGroupInfo.PossibleSlots
	slot := possibleSlots[rand.Intn(len(possibleSlots))]
	exam.day = slot.DayNumber
	exam.slot = slot.SlotNumber
}

type Plan struct {
	exams   []*Exam
	fitness float64
}

func (plan *Plan) randomPlan() {
	for _, exam := range plan.exams {
		exam.randomSlot()
	}
}

func (plan *Plan) validate() bool {
	examsMap := make(map[int]*Exam)
	for _, exam := range plan.exams {
		examsMap[exam.examGroup.ExamGroupCode] = exam
	}

	for _, exam1 := range plan.exams {
		if exam1.examGroup.ExamGroupInfo.NotPlannedByMe {
			continue
		}
		for _, conflict := range exam1.examGroup.ExamGroupInfo.Conflicts {
			exam2, ok := examsMap[conflict.ExamGroupCode]
			if ok {
				if exam2.examGroup.ExamGroupInfo.NotPlannedByMe {
					if exam1.examGroup.ExamGroupCode != exam2.examGroup.ExamGroupCode {
						if exam1.day == exam2.day {
							if exam1.slot-exam2.slot == 1 || exam1.slot == exam2.slot {
								return false
							}
						}
					}
				}
			}
		}
	}
	return true
}

// func (algorithm *Algorithm) moveSlotsToFront(plan *Plan) *Plan {
// 	planfitness := plan.fitness
// 	for _, exam := range plan.exams {
// 		day := exam.day
// 		slot := exam.slot
// 		for i := 1; i < day; i++ {
// 			exam.day = i
// 			for j := 1; j < slot; j++ {
// 				exam.slot = j
// 				if algorithm.evaluatePlan(plan) >= planfitness {
// 					day = i
// 					slot = j
// 					planfitness = plan.fitness
// 				}
// 			}
// 		}
// 		exam.slot = slot
// 		exam.day = day
// 		plan.fitness = planfitness
// 	}
// 	return plan
// }
