package generateplan

import "math/rand"

type Selection int

const (
	FitnessProportial Selection = iota
	StochasticUniversal
	RankedBased
)

// FitnessProportial

func (algorithm *Algorithm) fitness_proportional_selection() []*Plan {
	selection := []*Plan{}

	for i := 0; i < algorithm.config.Population; i++ {
		selection = append(selection, algorithm.selectPlanFitnessBased(rand.Float64()))
	}

	return selection
}

func (algorithm *Algorithm) selectPlanFitnessBased(selector float64) *Plan {
	var aPropability float64 = 0
	for _, plan := range algorithm.generation.plans {
		aPropability += plan.fitness / algorithm.generation.fitness
		if selector <= aPropability {
			return plan
		}
	}
	return nil
}

// StochasticUniversal

func (algorithm *Algorithm) stochastic_universal_selection() []*Plan {
	selection := []*Plan{}
	selector := rand.Float64()

	for i := 0; i < algorithm.config.Population; i++ {
		selection = append(selection, algorithm.selectPlanFitnessBased(selector))
		selector += (1 / float64(algorithm.config.Population))
		if selector >= 1 {
			selector -= 1
		}
	}

	return selection
}

// RankedBased

func (algorithm *Algorithm) ranked_based_selection() []*Plan {
	rankedGeneration := algorithm.generation.sortPlans()
	selection := []*Plan{}

	for i := 0; i < algorithm.config.Population; i++ {
		selection = append(selection, algorithm.selectPlanRankBased(rand.Float64(), rankedGeneration))
	}

	return selection
}

func (generation *Generation) sortPlans() *Generation {
	newGen := newGeneration()
	for _, plan := range generation.plans {
		newGen.plans = append(newGen.plans, plan)

		for i := len(newGen.plans) - 1; i > 0; i-- {
			if newGen.plans[i].fitness > newGen.plans[i-1].fitness {
				newGen.plans[i-1], newGen.plans[i] = newGen.plans[i], newGen.plans[i-1]
			}
		}
	}
	return newGen
}

func (algorithm *Algorithm) selectPlanRankBased(selector float64, rankedGeneration *Generation) *Plan {
	var aPropability float64 = 0
	for i, plan := range rankedGeneration.plans {
		aPropability += 1 / float64(algorithm.config.Population) * (algorithm.config.SelectivePressure -
			(2*algorithm.config.SelectivePressure-2)*(float64(i)/float64(algorithm.config.Population-1)))
		if selector <= aPropability {
			return plan
		}
	}
	return nil
}
