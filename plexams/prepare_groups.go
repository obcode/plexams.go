package plexams

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrepareExamsGroups() error {
	ctx := context.Background()
	examsWithRegsSlice, err := p.ExamsWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams with regs")
	}

	examsWithRegs := make(map[int]*model.ExamWithRegs)
	for _, exam := range examsWithRegsSlice {
		examsWithRegs[exam.Ancode] = exam
	}

	constraintsSlice, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams with constraints")
	}

	constraints := make(map[int]*model.Constraints)
	for _, constraint := range constraintsSlice {
		constraints[constraint.Ancode] = constraint
	}

	ancodesToPlan, err := p.GetZpaAnCodesToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams with constraints")
	}

	ancodes := make([]int, 0, len(ancodesToPlan))

	for _, ancode := range ancodesToPlan {
		ancodes = append(ancodes, ancode.Ancode)
	}

	sort.Ints(ancodes)

	examGroupCode := 0
	ancodesAlreadyInGroup := make([]int, 0)
	groups := make([]*model.ExamGroup, 0)
	for _, ancode := range ancodes {

		log.Debug().Int("ancode", ancode).Msg("preparing group for ancode")

		if ancodeAlreadyInGroup(ancode, ancodesAlreadyInGroup) {
			continue
		}

		// find all ancodes in one group
		ancodesToGroup := []int{ancode}

	OUTER:
		for {
			ancodesToGroupBefore := ancodesToGroup
			for _, ancodeInGroup := range ancodesToGroup {
				if otherConstraints := constraints[ancodeInGroup]; otherConstraints != nil {
					sameSlot := otherConstraints.SameSlot
					if sameSlot != nil {
						ancodesToGroup = append(ancodesToGroup, sameSlot...)
					}
				}
				ancodesToGroup = removeDuplicates(ancodesToGroup)
				if len(ancodesToGroupBefore) == len(ancodesToGroup) {
					break OUTER
				}
			}
		}

		// construct Group
		examGroupCode++

		sort.Ints(ancodesToGroup)

		examsInGroup := make([]*model.ExamToPlan, 0, len(ancodesToGroup))
		for _, ancode := range ancodesToGroup {
			toPlan := false
			for _, ancodeToPlan := range ancodes {
				if ancodeToPlan == ancode {
					toPlan = true
					break
				}
			}
			if toPlan {
				exam := examsWithRegs[ancode]
				constraints := constraints[ancode]
				examsInGroup = append(examsInGroup, &model.ExamToPlan{
					Exam:        exam,
					Constraints: constraints,
				})
			}
		}

		notPlannedByMe := false
		var excludeDays []int
		var possibleDays []int
		var fixedDay *int
		studentRegs := 0
		programs := make([]string, 0)
		maxDuration := 0

		for _, exam := range examsInGroup {
			if exam.Constraints != nil {
				notPlannedByMe = notPlannedByMe || exam.Constraints.NotPlannedByMe
				for _, date := range exam.Constraints.ExcludeDays {
					if excludeDays == nil {
						excludeDays = []int{p.dateToDay(date)}
					} else {
						excludeDays = append(excludeDays, p.dateToDay(date))
					}
				}
				for _, date := range exam.Constraints.PossibleDays {
					if possibleDays == nil {
						possibleDays = []int{p.dateToDay(date)}
					} else {
						possibleDays = append(possibleDays, p.dateToDay(date))
					}
				}
				if exam.Constraints.FixedDay != nil {
					fixedDayExam := p.dateToDay(exam.Constraints.FixedDay)

					if fixedDay != nil && *fixedDay != fixedDayExam {
						log.Error().Int("ancode", ancode).Msg("different fixed days in exam group")
						os.Exit(1)
					}
					fixedDay = &fixedDayExam
				}
			}

			for _, studRegs := range exam.Exam.StudentRegs {
				studentRegs += len(studRegs.StudentRegs)
				programs = append(programs, studRegs.Program)
			}

			if exam.Exam.ZpaExam.Duration > maxDuration {
				maxDuration = exam.Exam.ZpaExam.Duration
			}

		}

		if excludeDays != nil {
			excludeDays = removeDuplicates(excludeDays)
			sort.Ints(excludeDays)
		}

		if possibleDays != nil {
			possibleDays = removeDuplicates(possibleDays)
			sort.Ints(possibleDays)
		}

		programs = removeDuplicates(programs)
		sort.Strings(programs)

		// TODO: if excludeDays and possibleDays, do they still work togehter?
		// TODO: if fixedDay || fixedTime => does excludeDays and possibleDays hold?
		// TODO: only calculate possibleSlots?

		group := model.ExamGroup{
			ExamGroupCode: examGroupCode,
			Exams:         examsInGroup,
			ExamGroupInfo: &model.ExamGroupInfo{
				NotPlannedByMe: notPlannedByMe,
				ExcludeDays:    excludeDays,
				PossibleDays:   possibleDays,
				FixedDay:       fixedDay,
				FixedSlot:      nil,
				PossibleSlots:  nil,
				Conflicts:      nil,
				StudentRegs:    studentRegs,
				Programs:       programs,
				MaxDuration:    maxDuration,
				MaxDurationNta: nil, // TODO: calculate them
			},
		}

		calculatePossibleSlots(p.semesterConfig, &group)
		groups = append(groups, &group)

		ancodesAlreadyInGroup = append(ancodesAlreadyInGroup, ancodesToGroup...)
	}

	calculateExamGroupConflicts(groups)

	return p.dbClient.SaveExamGroups(ctx, groups)
}

func calculateExamGroupConflicts(groups []*model.ExamGroup) {
	// calculate a map: ancode -> examgoupcode
	groupCode := make(map[int]int)

	for _, group := range groups {
		eGcode := group.ExamGroupCode
		for _, exam := range group.Exams {
			groupCode[exam.Exam.Ancode] = eGcode
		}
	}

	// calculate examGroupConficts from exam conflicts and the map
	for _, group := range groups {
		conflictsMap := make(map[int]int) // examGroupCode -> count
		for _, examToPlan := range group.Exams {
			for _, conflictsProgram := range examToPlan.Exam.Conflicts {
				for _, conflicts := range conflictsProgram.Conflics {
					conflictGroup := groupCode[conflicts.AnCode]
					count, ok := conflictsMap[conflictGroup]
					if !ok {
						count = 0
					}
					conflictsMap[conflictGroup] = count + conflicts.NumberOfStuds
				}
			}
		}
		conflictKeys := make([]int, 0, len(conflictsMap))
		for k := range conflictsMap {
			conflictKeys = append(conflictKeys, k)
		}
		sort.Ints(conflictKeys)

		examGroupConficts := make([]*model.ExamGroupConflict, 0, len(conflictKeys))
		for _, code := range conflictKeys {
			examGroupConficts = append(examGroupConficts, &model.ExamGroupConflict{
				ExamGroupCode: code,
				Count:         conflictsMap[code],
			})
		}
		group.ExamGroupInfo.Conflicts = examGroupConficts
	}
}

func calculatePossibleSlots(semesterConfig *model.SemesterConfig, group *model.ExamGroup) {
	// FIXME: Implement me
}

func ancodeAlreadyInGroup(ancode int, ancodes []int) bool {
	for _, ancodeInGroup := range ancodes {
		if ancode == ancodeInGroup {
			return true
		}
	}
	return false
}

func removeDuplicates[T string | int](sliceList []T) []T {
	allKeys := make(map[T]bool)
	list := []T{}
	for _, item := range sliceList {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

func (p *Plexams) dateToDay(date *time.Time) int {
	for _, day := range p.semesterConfig.Days {
		if day.Date.Year() == date.Year() &&
			day.Date.Month() == date.Month() &&
			day.Date.Day() == date.Day() {
			return day.Number
		}
	}

	return -1
}
