package generateplan

import (
	"math"
	"math/rand"
)

type Generation struct {
	plans   []*Plan
	fitness float64
}

func newGeneration() *Generation {
	return &Generation{plans: []*Plan{}}
}

func (algorithm *Algorithm) initialGeneration() {
	firstGen := newGeneration()

	for i := 0; i < algorithm.config.Population; i++ {
		newPlan := algorithm.newPlan()
		newPlan.randomPlan()
		firstGen.plans = append(firstGen.plans, newPlan)
	}
	algorithm.generation = firstGen
	algorithm.evaluateGeneration()
}

func (algorithm *Algorithm) nextGeneration() {
	// log.Debug().Float64("fitness", algorithm.bestPlan.fitness).Msg("next generation")

	nextGen := newGeneration()

	var selection []*Plan

	switch algorithm.config.Selection {
	case FitnessProportial:
		selection = algorithm.fitness_proportional_selection()
	case StochasticUniversal:
		selection = algorithm.stochastic_universal_selection()
	case RankedBased:
		selection = algorithm.ranked_based_selection()
	}

	for len(selection) > 0 {
		parent1, parent2 := selectPartents(&selection)

		var newPlan1, newPlan2 *Plan

		switch algorithm.config.Recombination {
		case OnePointCrossover:
			newPlan1, newPlan2 = algorithm.one_point_crossover(parent1, parent2)
		case UniformCrossover:
			newPlan1, newPlan2 = algorithm.uniform_crossover(parent1, parent2)
		}

		algorithm.mutate(newPlan1)
		algorithm.mutate(newPlan2)

		nextGen.plans = append(nextGen.plans, []*Plan{newPlan1, newPlan2}...)

	}
	algorithm.generation = nextGen
	algorithm.evaluateGeneration()
}

func selectPartents(selection *[]*Plan) (*Plan, *Plan) {
	s := *selection

	selector := rand.Intn(len(s))
	parent1 := s[selector]
	s = append(s[:selector], s[selector+1:]...)

	selector = rand.Intn(len(s))
	parent2 := s[selector]
	s = append(s[:selector], s[selector+1:]...)

	*selection = s
	return parent1, parent2
}

func (algorithm *Algorithm) evaluateGeneration() {
	var fitness float64
	for _, plan := range algorithm.generation.plans {
		planFitness := algorithm.evaluatePlan(plan)
		if algorithm.bestPlan == nil {
			algorithm.bestPlan = plan

		} else if planFitness > algorithm.bestPlan.fitness {
			algorithm.bestPlan = plan
		}
		fitness += planFitness
	}
	algorithm.generation.fitness = fitness
}

func (algorithm *Algorithm) evaluatePlan(plan *Plan) float64 {
	examsMap := make(map[int]*Exam)
	for _, exam := range plan.exams {
		examsMap[exam.examGroup.ExamGroupCode] = exam
	}

	sValue, pValue := 0, 0

	for _, exam1 := range plan.exams {
		studentsPerSlot := exam1.examGroup.ExamGroupInfo.StudentRegs
		examsPerSlot := 1
		for _, exam2 := range plan.exams {
			if exam1.examGroup.ExamGroupInfo.NotPlannedByMe &&
				exam2.examGroup.ExamGroupInfo.NotPlannedByMe {
				continue
			}
			if exam1.examGroup.ExamGroupCode != exam2.examGroup.ExamGroupCode &&
				exam1.day == exam2.day && exam1.slot == exam2.slot {
				studentsPerSlot += exam2.examGroup.ExamGroupInfo.StudentRegs
				examsPerSlot++
			}
		}

		// TODO: Think about
		// if studentsPerSlot > CapasityStudent && examsPerSlot > 1 {
		// 	sValue += (studentsPerSlot - CapasityStudent) / examsPerSlot
		// }

		for _, conflict := range exam1.examGroup.ExamGroupInfo.Conflicts {
			exam2, ok := examsMap[conflict.ExamGroupCode]
			if ok { // otherwise the exam group code is not in plan
				if exam1.examGroup.ExamGroupCode != exam2.examGroup.ExamGroupCode {
					w := 0
					if exam1.day == exam2.day { // Prüfungen am gleichen Tag
						w = algorithm.validation.sameDay
						if math.Abs(float64(exam1.slot-exam2.slot)) >= 1 { //Prüfungen im gleichen Slot oder in aufeinanderfolgenden
							w = algorithm.validation.sameOrAdjacentSlots
						}
					} else if math.Abs(float64(exam1.day-exam2.day)) == 1 { //Prüfungen an aufeinanderfolgenden Tagen
						w = algorithm.validation.adjacentDays
					}
					pValue += w * exam1.examGroup.ExamGroupInfo.StudentRegs
				}
				// } else {
				// 	log.Debug().Int("exam group", conflict.ExamGroupCode).Msg("conflicting exam group is not part of the plan")
			}
		}
	}
	pValue /= 2
	plan.fitness = 1 / float64(1+sValue+pValue)
	return plan.fitness
}
