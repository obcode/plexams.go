package plexams

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) initPrepareExamGroups(ctx context.Context) (ancodesToPlan []int, examsWithRegs map[int]*model.ExamWithRegs, constraints map[int]*model.Constraints, err error) {
	examsWithRegsSlice, err := p.ExamsWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams with regs")
	}

	examsWithRegs = make(map[int]*model.ExamWithRegs)
	for _, exam := range examsWithRegsSlice {
		examsWithRegs[exam.Ancode] = exam
	}

	constraintsSlice, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams with constraints")
	}

	constraints = make(map[int]*model.Constraints)
	for _, constraint := range constraintsSlice {
		constraints[constraint.Ancode] = constraint
	}

	ancodesToPlanAncodes, err := p.GetZpaAnCodesToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams with constraints")
	}

	ancodesToPlan = make([]int, 0, len(ancodesToPlanAncodes))

	for _, ancode := range ancodesToPlanAncodes {
		ancodesToPlan = append(ancodesToPlan, ancode.Ancode)
	}

	sort.Ints(ancodesToPlan)

	return
}

func (p *Plexams) PrepareExamGroups() error {
	ctx := context.Background()
	ancodes, examsWithRegs, constraints, err := p.initPrepareExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot init prepare exam groups")
	}

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

		groups = append(groups, p.prepareExamGroup(examGroupCode, ancodesToGroup, ancodes, examsWithRegs, constraints))

		ancodesAlreadyInGroup = append(ancodesAlreadyInGroup, ancodesToGroup...)
	}

	calculateExamGroupConflicts(groups)

	return p.dbClient.SaveExamGroups(ctx, groups)
}

func (p *Plexams) PrepareExamGroup(ancodesToGroup []int) error {
	ctx := context.Background()
	ancodes, examsWithRegs, constraints, err := p.initPrepareExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot init prepare exam groups")
		return err
	}

	examGroupCode, err := p.dbClient.GetNextExamGroupCode(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get next exam group code")
		return err
	}

	examGroup := p.prepareExamGroup(examGroupCode, ancodesToGroup, ancodes, examsWithRegs, constraints)

	// conflicts
	// recalculate all conflicts
	// TODO: alte Konflikte sind nicht upgedatet, weil die neuen Prüfungen nicht in exams to plan waren
	examGroups, err := p.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get other exam groups")
		return err
	}

	examGroups = append(examGroups, examGroup)
	calculateExamGroupConflicts(examGroups)

	return p.dbClient.SaveExamGroups(ctx, examGroups)
}

func (p *Plexams) prepareExamGroup(examGroupCode int, ancodesToGroup, ancodesToPlan []int, examsWithRegs map[int]*model.ExamWithRegs, constraints map[int]*model.Constraints) *model.ExamGroup {
	examsInGroup := make([]*model.ExamToPlan, 0, len(ancodesToGroup))
	for _, ancode := range ancodesToGroup {
		toPlan := false
		for _, ancodeToPlan := range ancodesToPlan {
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
					log.Error().Msg("different fixed days in exam group")
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

	// TODO: if excludeDays and possibleDays, do they still work together?
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

	setPossibleSlots(p.semesterConfig, &group)

	return &group
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
				for _, conflicts := range conflictsProgram.Conflicts {
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
			if k != group.ExamGroupCode {
				conflictKeys = append(conflictKeys, k)
			}
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

func setPossibleSlots(semesterConfig *model.SemesterConfig, group *model.ExamGroup) {
	possibleSlotsPerExam := make([][]*model.Slot, 0)
	for _, exam := range group.Exams {
		possibleSlotsPerExam = append(possibleSlotsPerExam,
			CalculatedAllowedSlots(semesterConfig.Slots, semesterConfig.GoSlots, exam.IsGO(), exam.Constraints))
	}
	group.ExamGroupInfo.PossibleSlots = mergeAllowedSlots(possibleSlotsPerExam)
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
