package generateplan

import (
	"errors"
	"math"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

type AlgorithmConfig struct {
	Population          int
	MutationPropability float64
	Selection           Selection
	Recombination       Recombination
	SelectivePressure   float64 // = 1.3 // only for ranked_based_selection
	NoImprovement       int
	CapacityStudent     int
}

type Algorithm struct {
	config         *AlgorithmConfig
	validation     *Validation
	generation     *Generation
	bestPlan       *Plan
	semesterConfig *model.SemesterConfig
	examGroups     []*model.ExamGroup
}

type Validation struct {
	sameDay             int
	sameOrAdjacentSlots int
	adjacentDays        int
}

func (algorithm *Algorithm) newPlan() *Plan {
	newPlan := &Plan{exams: []*Exam{}}
	for _, examGroup := range algorithm.examGroups {
		newPlan.exams = append(newPlan.exams, newExam(examGroup))
	}
	return newPlan
}

func InitalizeAlgorithm(semesterConfig *model.SemesterConfig, examGroups []*model.ExamGroup, pop int, pm float64, sel Selection, rec Recombination, sp float64) *Algorithm {
	return &Algorithm{
		config: &AlgorithmConfig{
			Population:          pop,
			MutationPropability: 1 - math.Pow(1-pm, 1./float64(len(examGroups))),
			Selection:           sel,
			Recombination:       rec,
			SelectivePressure:   sp,
			NoImprovement:       100,
			CapacityStudent:     60,
		},
		validation: &Validation{
			sameDay:             5,
			sameOrAdjacentSlots: 10,
			adjacentDays:        1,
		},
		semesterConfig: semesterConfig,
		examGroups:     examGroups,
	}
}

func (algorithm *Algorithm) reset() {
	algorithm.bestPlan = nil
}

func (algorithm *Algorithm) NRuns(n int) ([]*model.PlanEntry, error) {
	var bestPlan *Plan

	for i := 0; i < n; i++ {
		newPlan := algorithm.run()
		if bestPlan == nil || bestPlan.fitness < newPlan.fitness {
			bestPlan = newPlan
		}
		log.Debug().Int("i", i).Float64("fitness", bestPlan.fitness).Msg("best plan found in run")
		algorithm.reset()
	}

	// plan := algorithm.moveSlotsToFront(bestPlan)
	plan := bestPlan
	if !plan.validate() {
		return nil, errors.New("no Valid Plan found")
	}

	var exportPlan []*model.PlanEntry
	for _, exam := range plan.exams {
		exportPlan = append(exportPlan,
			&model.PlanEntry{DayNumber: exam.day, SlotNumber: exam.slot, Ancode: exam.examGroup.ExamGroupCode, Locked: false})
	}
	return exportPlan, nil
}

func (algorithm *Algorithm) run() *Plan {
	algorithm.initialGeneration()

	var bestGenFitness float64 = 0
	counter := 0
	for counter < algorithm.config.NoImprovement {
		algorithm.nextGeneration()
		counter++
		if algorithm.generation.fitness > bestGenFitness {
			bestGenFitness = algorithm.generation.fitness
			counter = 0
		}
	}
	return algorithm.bestPlan
}
