package plexams

import (
	"context"
	"math"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigcalc"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) InvigilatorsWithReq(ctx context.Context) ([]*model.Invigilator, error) {
	teachers, err := p.getInvigilators(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get teachers")
		return nil, err
	}

	isNotInvigilator, constraintsMap, err := p.notInvigilating(ctx)
	if err != nil {
		return nil, err
	}

	invigilators := make([]*model.Invigilator, 0, len(teachers))
	for _, teacher := range teachers {
		if isNotInvigilator(teacher.ID) {
			log.Debug().Str("name", teacher.Shortname).Msg("is not invigilator")
			continue
		}

		invigilator, err := p.buildInvigilator(ctx, teacher, constraintsMap[teacher.ID])
		if err != nil {
			return nil, err
		}
		invigilators = append(invigilators, invigilator)
	}

	return invigilators, nil
}

// InvigilatorsExcludedByConfig returns the invigilators who WOULD do invigilation
// duty (they are in the pool and their computed factor is > 0) but are excluded
// solely because invigilatorConstraints.<id>.isNotInvigilator is set in the
// semester config. People who are out anyway (factor 0, e.g. full free semester)
// are not returned here.
func (p *Plexams) InvigilatorsExcludedByConfig(ctx context.Context) ([]*model.Invigilator, error) {
	teachers, err := p.getInvigilators(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get teachers")
		return nil, err
	}

	isNotInvigilator, constraintsMap, err := p.notInvigilating(ctx)
	if err != nil {
		return nil, err
	}

	excluded := make([]*model.Invigilator, 0)
	for _, teacher := range teachers {
		if !isNotInvigilator(teacher.ID) {
			continue
		}
		invigilator, err := p.buildInvigilator(ctx, teacher, constraintsMap[teacher.ID])
		if err != nil {
			return nil, err
		}
		if invigilator.Requirements != nil && invigilator.Requirements.Factor > 0 {
			excluded = append(excluded, invigilator)
		}
	}

	return excluded, nil
}

// buildInvigilator assembles the full invigilator (requirements, factor, exam
// times, config time windows) for one teacher, independent of whether the
// teacher is excluded by config.
func (p *Plexams) buildInvigilator(ctx context.Context, teacher *model.Teacher, constraints *model.InvigilatorConstraints) (*model.Invigilator, error) {
	reqs, err := p.dbClient.GetInvigilatorRequirements(ctx, teacher.ID)
	if err != nil {
		log.Error().Err(err).Str("teacher", teacher.Shortname).Msg("cannot get requirements for teacher")
	}

	// Requirements aus dem ZPA können noch fehlen. Dann planen wir mit
	// Standard-Anforderungen (Vollzeit, keine angerechneten Beiträge) und
	// merken uns über FromZpa, dass die echten Anforderungen noch fehlen.
	fromZPA := reqs != nil
	if reqs == nil {
		reqs = &zpa.SupervisorRequirements{PartTime: 1.0}
	}

	// Additional per-invigilator constraints come from the DB (managed via the
	// GUI), merged on top of the ZPA requirements.
	var timeWindows []*model.InvigilationTimeWindow
	if constraints != nil {
		timeWindows = constraints.TimeWindows
	}

	// ExcludedDates: the (≤3) whole days from the ZPA (stored as "02.01.06"
	// strings) plus the additional ones from the DB constraints.
	loc, _ := time.LoadLocation("Europe/Berlin")
	excludedDates := make([]*time.Time, 0, len(reqs.ExcludedDates))
	for _, day := range reqs.ExcludedDates {
		t, err := time.ParseInLocation("02.01.06", day, loc)
		if err != nil {
			log.Error().Err(err).Str("day", day).Msg("cannot parse date")
		} else {
			excludedDates = append(excludedDates, &t)
		}
	}
	if constraints != nil {
		for i := range constraints.ExcludedDates {
			d := constraints.ExcludedDates[i]
			excludedDates = append(excludedDates, &d)
		}
	}

	excludedDays := p.datesToDay(excludedDates)

	examTimes := make([]*model.ExamTime, 0)
	examStarttimes := make([]*time.Time, 0)
	exams, err := p.AssembledExamsForExamer(ctx, teacher.ID)
	if err != nil {
		log.Error().Err(err).Str("name", teacher.Shortname).Msg("cannit get exams by main examer")
	} else {
		for _, exam := range exams {
			planEntry, err := p.dbClient.PlanEntry(ctx, exam.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get plan entry for ancode")
			}
			if planEntry != nil {
				// Slot (0,0) is the marker for exams outside the exam period
				// (außerhalb des Prüfungszeitraums); they have no real slot time,
				// so they contribute no exam time for the invigilator.
				if planEntry.DayNumber == 0 && planEntry.SlotNumber == 0 {
					continue
				}
				starttimePtr, err := p.GetStarttime(planEntry.DayNumber, planEntry.SlotNumber)
				if err != nil {
					log.Error().Err(err).Str("name", teacher.Shortname).
						Int("ancode", exam.Ancode).
						Int("dayNumber", planEntry.DayNumber).Int("slotNumber", planEntry.SlotNumber).
						Msg("plan entry does not map to a valid slot, skipping exam time for invigilator")
					continue
				}
				starttime := *starttimePtr
				endtime := starttime.Add(time.Duration(exam.MaxDuration) * time.Minute)
				examTimes = append(examTimes, &model.ExamTime{
					From:  starttime,
					Until: endtime,
				})
				examStarttimes = append(examStarttimes, &starttime)
			}
		}
	}

	factor := 1.0 * reqs.PartTime

	if reqs.OvertimeThisSemester != 0 {
		factor *= reqs.OvertimeThisSemester
	}

	if reqs.FreeSemester == 0.5 {
		factor *= 0.5
	}

	if reqs.FreeSemester == 1.0 ||
		reqs.OvertimeLastSemester != 0 && reqs.FreeSemester == 0.5 {
		factor = 0.0
	}

	log.Debug().Str("name", teacher.Shortname).Float64("faktor", factor).
		Msg("Faktor für Aufsichten")

	return &model.Invigilator{
		Teacher: teacher,
		Requirements: &model.InvigilatorRequirements{
			ExcludedDates:          excludedDates,
			ExcludedDays:           excludedDays,
			ExamTimes:              examTimes,
			ExamDays:               p.datesToDay(examStarttimes),
			PartTime:               reqs.PartTime,
			OralExamsContribution:  reqs.OralExamsContribution,
			LiveCodingContribution: reqs.LivecodingContribution,
			MasterContribution:     reqs.MasterContribution,
			FreeSemester:           reqs.FreeSemester,
			OvertimeLastSemester:   reqs.OvertimeLastSemester,
			OvertimeThisSemester:   reqs.OvertimeThisSemester,
			AllContributions:       reqs.OralExamsContribution + reqs.LivecodingContribution + reqs.MasterContribution,
			Factor:                 factor,
			FromZpa:                fromZPA,
			TimeWindows:            timeWindows,
		},
	}, nil
}

func (p *Plexams) datesToDay(dates []*time.Time) []int {
	days := set.NewSet[int]()
	for _, date := range dates {
		for _, day := range p.semesterConfig.Days {
			if day.Date.Month() == date.Month() && day.Date.Day() == date.Day() {
				days.Add(day.Number)
			}
		}
	}
	daysSlice := days.ToSlice()
	sort.Ints(daysSlice)
	return daysSlice
}

func (p *Plexams) GetInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	todos, err := p.dbClient.GetInvigilationTodos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilation todos")
		return nil, err
	}
	if todos == nil {
		return p.PrepareInvigilationTodos(ctx)
	}

	err = p.AddInvigilatorsToInvigilationTodos(ctx, todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot add invigilators to invigilation todos")
		return nil, err
	}

	err = p.dbClient.CacheInvigilatorTodos(ctx, todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilation todos")
		return nil, err
	}

	return todos, nil
}

func (p *Plexams) PrepareInvigilationTodos(ctx context.Context) (*model.InvigilationTodos, error) {
	selfInvigilations, err := p.MakeSelfInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get self invigilations")
	}

	todos := model.InvigilationTodos{}

	for _, slot := range p.semesterConfig.Slots {
		roomsInSlot, err := p.PlannedRoomsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("cannot get rooms for slot")
		} else {
			if len(roomsInSlot) == 0 {
				continue
			}
			roomMap := make(map[string]int)
		OUTER:
			for _, room := range roomsInSlot {
				for _, selfInvigilation := range selfInvigilations {
					if selfInvigilation.Slot.DayNumber == slot.DayNumber &&
						selfInvigilation.Slot.SlotNumber == slot.SlotNumber &&
						*selfInvigilation.RoomName == room.RoomName {
						log.Debug().Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Str("room", room.RoomName).
							Msg("found self invigilation")
						continue OUTER
					}
				}
				maxDuration, ok := roomMap[room.RoomName]
				if !ok || maxDuration < room.Duration {
					roomMap[room.RoomName] = room.Duration
				}
			}

			for _, maxDuration := range roomMap {
				todos.SumExamRooms += maxDuration
			}
			todos.SumReserve += 60 // FIXME: Maybe some other time? half of max duration in slot?
		}
	}

	reqs, err := p.InvigilatorsWithReq(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators with regs")
	}

	todos.Invigilators = reqs

	for _, invigilator := range reqs {
		if invigilator.Requirements != nil {
			todos.SumOtherContributions += invigilator.Requirements.AllContributions
		}
	}

	todos.InvigilatorCount = len(reqs)
	adjustedInvigilatorCount := 0.0

	for _, invigilator := range reqs {
		if invigilator.Requirements != nil {
			adjustedInvigilatorCount += invigilator.Requirements.Factor
		}
	}

	todos.TodoPerInvigilator = int(math.Ceil(float64(todos.SumExamRooms+todos.SumReserve+todos.SumOtherContributions) / adjustedInvigilatorCount))

	// Verteile nur die tatsächlich zu leistenden Minuten (SumExamRooms +
	// SumReserve) faktorgewichtet auf die Aufsichten und rechne dabei die bereits
	// erbrachten Beiträge an. Wer mehr beigetragen hat als seinen Anteil, fällt
	// komplett heraus (Über-Beitrag ist "Schicksal" und lässt sich nicht auf die
	// anderen umlegen) -- siehe fairInvigilationTargets.
	var targets map[int]int
	var enough map[int]bool
	todos.TodoPerInvigilatorOvertimeCutted, todos.SumOtherContributionsOvertimeCutted, targets, enough =
		invigcalc.FairTargets(todos.SumExamRooms+todos.SumReserve, reqs)

	for _, invigilator := range todos.Invigilators {
		invigilationsForInvigilator, err := p.dbClient.InvigilationsForInvigilator(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("invigilator", invigilator.Teacher.Shortname).
				Msg("cannot get invigilations")
		}

		invigilator.Todos = invigcalc.Todos(invigilationsForInvigilator,
			targets[invigilator.Teacher.ID], enough[invigilator.Teacher.ID])
	}

	err = p.dbClient.CacheInvigilatorTodos(ctx, &todos)
	if err != nil {
		log.Error().Err(err).Msg("cannot cache invigilation todos")
		return &todos, err
	}

	return &todos, nil
}

func (p *Plexams) AddInvigilatorsToInvigilationTodos(ctx context.Context, todos *model.InvigilationTodos) error {
	reqs, err := p.InvigilatorsWithReq(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators with regs")
		return err
	}

	todos.Invigilators = reqs

	// Re-derive the fair targets from the cached work minutes and the current
	// requirements, so the per-invigilator targets sum to exactly the work to be
	// covered (see fairInvigilationTargets).
	todoPerInvigilator, countedContributions, targets, enough :=
		invigcalc.FairTargets(todos.SumExamRooms+todos.SumReserve, reqs)
	todos.TodoPerInvigilatorOvertimeCutted = todoPerInvigilator
	todos.SumOtherContributionsOvertimeCutted = countedContributions

	for _, invigilator := range todos.Invigilators {
		invigilationsForInvigilator, err := p.dbClient.InvigilationsForInvigilator(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("invigilator", invigilator.Teacher.Shortname).
				Msg("cannot get invigilations")
			return err
		}

		invigilator.Todos = invigcalc.Todos(invigilationsForInvigilator,
			targets[invigilator.Teacher.ID], enough[invigilator.Teacher.ID])
	}

	return nil
}

func (p *Plexams) InvigilatorsForDay(ctx context.Context, day int) (*model.InvigilatorsForDay, error) {
	invigilationTodos, err := p.dbClient.GetInvigilationTodos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilation todos")
		return nil, err
	}

	want := make([]*model.Invigilator, 0)
	can := make([]*model.Invigilator, 0)

	for _, invigilator := range invigilationTodos.Invigilators {
		wantDay, canDay := dayOkForInvigilator(day, invigilator)
		if wantDay {
			want = append(want, invigilator)
		} else if canDay {
			can = append(can, invigilator)
		}
	}

	return &model.InvigilatorsForDay{
		Want: want,
		Can:  can,
	}, nil
}

func dayOkForInvigilator(day int, invigilator *model.Invigilator) (wantDay, canDay bool) {
	// day in exlude days?
	if invigilator.Requirements != nil {
		for _, excludedDay := range invigilator.Requirements.ExcludedDays {
			if day == excludedDay {
				return false, false
			}
		}
		for _, examDay := range invigilator.Requirements.ExamDays {
			if day == examDay {
				return true, true
			}
		}
		for _, invigilationDay := range invigilator.Todos.InvigilationDays {
			if day == invigilationDay {
				return true, true
			}
		}
	}
	return false, true
}

func (p *Plexams) PrepareSelfInvigilation() error {
	ctx := context.Background()
	selfInvigilations, err := p.MakeSelfInvigilations(ctx)
	if err != nil {
		return err
	}

	toSave := make([]interface{}, 0, len(selfInvigilations))
	for _, invig := range selfInvigilations {
		log.Debug().Interface("invigilation", invig).Msg("adding invigilation to slice")
		toSave = append(toSave, invig)
	}

	log.Debug().Interface("ivigilations", toSave).Msg("saving invigilations")

	return p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "invigilations_self"), toSave)
}

func (p *Plexams) MakeSelfInvigilations(ctx context.Context) ([]*model.Invigilation, error) {
	invigilators, err := p.InvigilatorsWithReq(ctx)
	if err != nil || invigilators == nil {
		log.Error().Err(err).Msg("cannot get invigilators")
		return nil, err
	}

	log.Debug().Interface("invigilators", invigilators).Msg("got invigilators")

	invigilatorMap := make(map[int]*model.Invigilator)
	for _, invigilator := range invigilators {
		invigilatorMap[invigilator.Teacher.ID] = invigilator
	}

	type key struct {
		roomName string
		day      int
		slot     int
	}

	invigilationsMap := make(map[key][]*model.Invigilation)

	for _, slot := range p.semesterConfig.Slots {
		examsInSlot, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("cannot get exams in slot")
			continue
		}
		examerWithExams := make(map[int][]*model.PlannedExam)
	OUTER:
		for _, exam := range examsInSlot {
			invigilator, ok := invigilatorMap[exam.ZpaExam.MainExamerID]

			if !ok {
				log.Debug().Str("name", exam.ZpaExam.MainExamer).Msg("ist keine Aufsicht")
				continue
			}
			if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
				continue
			}
			if len(exam.PlannedRooms) == 0 {
				continue
			}
			if invigilator.Requirements != nil {
				for _, day := range invigilator.Requirements.ExcludedDays {
					if day == exam.PlanEntry.DayNumber {
						log.Debug().Str("name", exam.ZpaExam.MainExamer).Interface("slot", exam.PlanEntry).
							Msg("Tag ist gesperrt für Aufsicht")
						continue OUTER
					}
				}
			}
			exams, ok := examerWithExams[exam.ZpaExam.MainExamerID]
			if !ok {
				examerWithExams[exam.ZpaExam.MainExamerID] = []*model.PlannedExam{exam}
			} else {
				examerWithExams[exam.ZpaExam.MainExamerID] = append(exams, exam)
			}
		}

		for examer, exams := range examerWithExams {
			roomNames := set.NewSet[string]()
			for _, exam := range exams {
				for _, room := range exam.PlannedRooms {
					roomNames.Add(room.RoomName)
				}
			}

			if roomNames.Cardinality() == 1 {
				log.Debug().Int("examerid", examer).Interface("room", roomNames).Interface("slot", slot).
					Msg("found self invigilation")
				key := key{
					roomName: roomNames.ToSlice()[0],
					day:      slot.DayNumber,
					slot:     slot.SlotNumber,
				}
				invigilationsForKey, ok := invigilationsMap[key]
				if !ok {
					invigilationsForKey = make([]*model.Invigilation, 0, 1)
				}
				invigilationsMap[key] = append(invigilationsForKey, &model.Invigilation{
					RoomName:           &roomNames.ToSlice()[0],
					Duration:           0, // FIXME: ?? self-invigilation does not count
					InvigilatorID:      examer,
					Slot:               slot,
					IsReserve:          false,
					IsSelfInvigilation: true,
				})
			}
		}

	}

	invigilations := make([]*model.Invigilation, 0)
	for _, invigs := range invigilationsMap {
		// if len(invigs) == 1 {
		// 	invigilations = append(invigilations, invigs...)
		// } else {
		if len(invigs) > 1 {
			log.Debug().Interface("invigs", invigs).Msg("found more self invigs")
		}
		// 	// TODO: find examer with most studs in room
		// 	// for _, invig := range invigs {

		// 	// }
		// }
		invigilations = append(invigilations, invigs[0])
	}

	log.Debug().Int("count", len(invigilations)).Msg("found self invigilations")

	return invigilations, nil
}

func (p *Plexams) GetInvigilatorForRoom(ctx context.Context, name string, day, time int) (*model.Teacher, error) {
	return p.dbClient.GetInvigilatorForRoom(ctx, name, day, time)
}

// TODO: rewrite me
func (p *Plexams) RoomsWithInvigilationsForSlot(ctx context.Context, day int, time int) (*model.InvigilationSlot, error) {
	rooms, err := p.PlannedRoomsInSlot(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).
			Msg("cannot get rooms for slot")
		return nil, err
	}

	if len(rooms) == 0 {
		return nil, nil // okay?
	}

	reserve, err := p.dbClient.GetInvigilatorInSlot(ctx, "reserve", day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).
			Msg("cannot get reserve for slot")
		return nil, err
	}

	// which rooms (and the reserve) are pre-planned in this slot
	prePlannedInvigilations, err := p.dbClient.PrePlannedInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre-planned invigilations")
		return nil, err
	}
	prePlannedRooms := make(map[string]bool)
	reservePrePlanned := false
	for _, ppi := range prePlannedInvigilations {
		if ppi.Day != day || ppi.Slot != time {
			continue
		}
		if ppi.RoomName == nil {
			reservePrePlanned = true
		} else {
			prePlannedRooms[*ppi.RoomName] = true
		}
	}

	slot := &model.InvigilationSlot{
		Reserve:               reserve,
		ReservePrePlanned:     reservePrePlanned,
		RoomsWithInvigilators: []*model.RoomWithInvigilator{},
	}

	roomMap := make(map[string][]*model.PlannedRoom)

	for _, room := range rooms {
		roomsForExam, ok := roomMap[room.RoomName]
		if !ok {
			roomsForExam = make([]*model.PlannedRoom, 0, 1)
		}
		roomMap[room.RoomName] = append(roomsForExam, room)
	}

	keys := make([]string, 0, len(roomMap))
	for k := range roomMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		roomsForExam := roomMap[name]
		invigilator, err := p.GetInvigilatorForRoom(ctx, name, day, time)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", time).Str("room", name).
				Msg("cannot get invigilator for rooms in slot")
		}

		roomAndExams := make([]*model.RoomAndExam, 0)
		maxDuration := 0
		studentCount := 0
		for _, roomForExam := range roomsForExam {
			exam, err := p.dbClient.GetZpaExamByAncode(ctx, roomForExam.Ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", roomForExam.Ancode).
					Msg("cannot get zpa exam")
				return nil, err
			}
			roomAndExams = append(roomAndExams, &model.RoomAndExam{
				Room: roomForExam,
				Exam: exam,
			})
			if roomForExam.Duration > maxDuration {
				maxDuration = roomForExam.Duration
			}
			studentCount += len(roomForExam.StudentsInRoom)
		}

		slot.RoomsWithInvigilators = append(slot.RoomsWithInvigilators, &model.RoomWithInvigilator{
			Name:         name,
			MaxDuration:  maxDuration,
			StudentCount: studentCount,
			RoomAndExams: roomAndExams,
			Invigilator:  invigilator,
			PrePlanned:   prePlannedRooms[name],
		})
	}
	return slot, nil
}

func (p *Plexams) Invigilator(ctx context.Context, room string, day int, time int) (*model.Teacher, error) {
	return p.dbClient.GetInvigilatorForRoom(ctx, room, day, time)
}
