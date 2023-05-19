package generateplan

import "math/rand"

type Recombination int

const (
	OnePointCrossover Recombination = iota
	UniformCrossover
)

func (algorithm *Algorithm) one_point_crossover(plan1, plan2 *Plan) (*Plan, *Plan) {
	coPoint := rand.Intn(len(plan1.exams))
	newPlan1 := algorithm.newPlan()
	newPlan2 := algorithm.newPlan()

	for i := range newPlan1.exams {
		if i <= coPoint {
			newPlan1.exams[i].day = plan1.exams[i].day
			newPlan1.exams[i].slot = plan1.exams[i].slot
			newPlan2.exams[i].day = plan2.exams[i].day
			newPlan2.exams[i].slot = plan2.exams[i].slot
		} else {
			newPlan1.exams[i].day = plan2.exams[i].day
			newPlan1.exams[i].slot = plan2.exams[i].slot
			newPlan2.exams[i].day = plan1.exams[i].day
			newPlan2.exams[i].slot = plan1.exams[i].slot
		}
	}

	return newPlan1, newPlan2
}

func (algorithm *Algorithm) uniform_crossover(plan1, plan2 *Plan) (*Plan, *Plan) {
	newPlan1 := algorithm.newPlan()
	newPlan2 := algorithm.newPlan()

	for i := range newPlan1.exams {
		if rand.Float64() < 0.5 {
			newPlan1.exams[i].day = plan1.exams[i].day
			newPlan1.exams[i].slot = plan1.exams[i].slot
			newPlan2.exams[i].day = plan2.exams[i].day
			newPlan2.exams[i].slot = plan2.exams[i].slot
		} else {
			newPlan1.exams[i].day = plan2.exams[i].day
			newPlan1.exams[i].slot = plan2.exams[i].slot
			newPlan2.exams[i].day = plan1.exams[i].day
			newPlan2.exams[i].slot = plan1.exams[i].slot
		}
	}
	return newPlan1, newPlan2
}

func (algorithm *Algorithm) mutate(plan *Plan) {
	for i := range plan.exams {
		if rand.Float64() < algorithm.config.MutationPropability {
			plan.exams[i].randomSlot()
		}
	}
}
