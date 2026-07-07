package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateInvigilatorRequirements(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilator-requirements", "validating invigilator requirements")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	v.step("recalculating todos")
	invigilationTodos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		return nil, err
	}

	for _, invigilator := range invigilationTodos.Invigilators {
		v.step("checking %s", invigilator.Teacher.Fullname)
		log.Debug().Str("name", invigilator.Teacher.Shortname).Msg("checking constraints")

		// days ok
		for _, invigilationDay := range invigilator.Todos.InvigilationDays {
			for _, excludedDay := range invigilator.Requirements.ExcludedDays {
				if invigilationDay == excludedDay {
					v.errorf(ref{InvigilatorID: ptr(invigilator.Teacher.ID)},
						"%s has invigilation on excluded day %d", invigilator.Teacher.Fullname, invigilationDay)
				}
			}
		}

		// nur ein Raum oder Reserve
		invigilationSlots := set.NewSet[int64]() // keyed by the absolute start (Unix)
		for _, invigilation := range invigilator.Todos.Invigilations {
			if invigilation.Starttime == nil {
				continue
			}
			startKey := invigilation.Starttime.Unix()
			if invigilationSlots.Contains(startKey) {
				v.errorf(ref{InvigilatorID: ptr(invigilator.Teacher.ID), Starttime: invigilation.Starttime},
					"%s has more than one invigilation at %s",
					invigilator.Teacher.Fullname, invigilation.Starttime.Format("02.01. 15:04"))
			}
			invigilationSlots.Add(startKey)

		}

		// wenn gleichzeitig Prüfung, dann nur self-invigilation
		exams, err := p.PlannedExamsByExamer(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("name", invigilator.Teacher.Shortname).Msg("cannot get exams")
		}

		for _, exam := range exams {
			for _, invigilation := range invigilator.Todos.Invigilations {
				if exam.PlanEntry != nil && exam.PlanEntry.Starttime != nil &&
					invigilation.Starttime != nil &&
					exam.PlanEntry.Starttime.Equal(*invigilation.Starttime) {
					when := invigilation.Starttime.Format("02.01. 15:04")
					if invigilation.IsReserve {
						v.errorf(ref{Ancode: ptr(exam.Constraints.Ancode), InvigilatorID: ptr(invigilator.Teacher.ID), Starttime: invigilation.Starttime},
							"%s has reserve invigilation during own exam %d. %s at %s",
							invigilator.Teacher.Fullname, exam.Constraints.Ancode, exam.ZpaExam.Module, when)
					}

					roomsForExam, err := p.dbClient.PlannedRoomsForAncode(ctx, exam.Ancode)
					rooms := set.NewSet[string]()
					for _, room := range roomsForExam {
						rooms.Add(room.RoomName)
					}

					if err != nil {
						log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get rooms for exam")
					} else {
						if rooms.Cardinality() > 1 {
							v.errorf(ref{Ancode: ptr(exam.Constraints.Ancode), InvigilatorID: ptr(invigilator.Teacher.ID), Starttime: invigilation.Starttime},
								"%s has invigilation during own exam with more than one room: %d. %s at %s: found rooms %v",
								invigilator.Teacher.Fullname, exam.Constraints.Ancode, exam.ZpaExam.Module, when, rooms)
						}
					}

				}
			}
		}

	}

	return v.finish(), nil
}
func (p *Plexams) ValidateInvigilationDups(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilation-duplicates", "validating invigilator duplicates")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	v.step("getting all invigilations")
	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all invigilations")
		return nil, err
	}

	type key struct {
		room  string
		start int64 // Unix seconds of the absolute start
	}

	invigilationsMap := make(map[key]*model.Invigilation)

	v.step("checking %d invigilations", len(invigilations))
	for _, invigilation := range invigilations {
		if invigilation.Starttime == nil {
			continue
		}
		var room string
		if invigilation.RoomName == nil {
			room = "null"
		} else {
			room = *invigilation.RoomName
		}
		key := key{
			room:  room,
			start: invigilation.Starttime.Unix(),
		}

		_, ok := invigilationsMap[key]
		if ok {
			v.errorf(ref{Room: invigilation.RoomName, InvigilatorID: ptr(invigilation.InvigilatorID), Starttime: invigilation.Starttime},
				"double entry for {roomname: %s, start: %s}",
				room, invigilation.Starttime.Format("02.01. 15:04"))
		} else {
			invigilationsMap[key] = invigilation
		}
	}

	return v.finish(), nil
}

// TODO: NTA- und Reserve-Aufsicht (wenn NTA) nicht im folgenden Slot einteilen!
func (p *Plexams) ValidateInvigilatorSlots(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilator-slots", "validating invigilator for all slots")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	// count rooms and reserves without and print number, keyed by calendar date
	// ("2006-01-02" sorts chronologically as a string).
	roomWithoutInvigilatorDay := make(map[string]int)
	slotWithoutReserveDay := make(map[string]int)
	maxInvigsMissingInOneSlot := make(map[string]int)
	dateByKey := make(map[string]time.Time)

	// all rooms and reserve max one invigilator
	for _, slot := range p.semesterConfig.Slots {
		invigsMissing := 0
		v.step("checking slot %s", slot.Starttime.Format("02.01. 15:04"))

		rooms, err := p.plannedRoomNamesAt(ctx, slot.Starttime)
		if err != nil {
			log.Error().Err(err).Time("start", slot.Starttime).Msg("cannot get rooms for")
		}
		if len(rooms) == 0 {
			continue
		}
		dayKey := slot.Starttime.Format("2006-01-02")
		dateByKey[dayKey] = slot.Starttime

		invigilations, err := p.invigilationsAt(ctx, "reserve", slot.Starttime)
		if err != nil {
			log.Error().Err(err).Time("start", slot.Starttime).Msg("cannot get reserve invigilator")
		}

		if len(invigilations) == 0 {
			slotWithoutReserveDay[dayKey]++
			invigsMissing++
		} else if len(invigilations) > 1 {
			v.errorf(ref{Starttime: &slot.Starttime},
				"more than one reserve invigilator at %s", slot.Starttime.Format("02.01. 15:04"))
		}

		for _, room := range rooms {
			invigilations, err := p.invigilationsAt(ctx, room, slot.Starttime)
			if err != nil {
				log.Error().Err(err).Time("start", slot.Starttime).Str("room", room).
					Msg("cannot get reserve invigilator")
			}
			if len(invigilations) == 0 {
				roomWithoutInvigilatorDay[dayKey]++
				invigsMissing++
			} else if len(invigilations) > 1 {
				v.warnf(ref{Room: ptr(room), Starttime: &slot.Starttime},
					"more than one invigilator for room %s at %s", room, slot.Starttime.Format("02.01. 15:04"))
			}
		}
		if invigsMissing > maxInvigsMissingInOneSlot[dayKey] {
			maxInvigsMissingInOneSlot[dayKey] = invigsMissing
		}
	}

	if len(roomWithoutInvigilatorDay) > 0 || len(slotWithoutReserveDay) > 0 {
		keySet := set.NewSet[string]()
		for k := range roomWithoutInvigilatorDay {
			keySet.Add(k)
		}
		for k := range slotWithoutReserveDay {
			keySet.Add(k)
		}
		keys := keySet.ToSlice()

		sort.Strings(keys)

		for _, dayKey := range keys {
			roomsWithoutInvig := roomWithoutInvigilatorDay[dayKey]
			slotsWithoutReserve := slotWithoutReserveDay[dayKey]

			if roomsWithoutInvig+slotsWithoutReserve > 0 {
				dayStart := dateByKey[dayKey]
				v.warnf(ref{Starttime: &dayStart},
					"Tag %s: %d open invigilations (%d max. in one slot), %d rooms without invigilator, %d slots without reserve",
					dateByKey[dayKey].Format("02.01."), roomsWithoutInvig+slotsWithoutReserve, maxInvigsMissingInOneSlot[dayKey], roomsWithoutInvig, slotsWithoutReserve)
			}
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateInvigilationsTimeDistance(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	timelag := p.generationTimelagMin(ctx)

	v := newValidation(reporter, "invigilations-time-distance",
		fmt.Sprintf("validating time lag of invigilations (%d minutes)", timelag))

	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	v.step("prepare invigilations")

	allInvigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all invigilations")
	}

	// Group the invigilations per invigilator and sort each group by absolute
	// start time. The old positional "consecutive slot pair per day" scan becomes
	// a time-interval rule: for two invigilations of the same person on the same
	// calendar day the later one must not begin before the earlier one ends plus
	// the required time lag.
	byInvigilator := make(map[int][]*model.Invigilation)
	for _, invigilation := range allInvigilations {
		if invigilation.Starttime == nil {
			continue
		}
		byInvigilator[invigilation.InvigilatorID] = append(byInvigilator[invigilation.InvigilatorID], invigilation)
	}

	invigilatorIDs := make([]int, 0, len(byInvigilator))
	for id := range byInvigilator {
		invigilatorIDs = append(invigilatorIDs, id)
	}
	sort.Ints(invigilatorIDs)

	for _, id := range invigilatorIDs {
		invigs := byInvigilator[id]
		sort.Slice(invigs, func(i, j int) bool {
			return invigs[i].Starttime.Before(*invigs[j].Starttime)
		})

		for i := 0; i+1 < len(invigs); i++ {
			inv1, inv2 := invigs[i], invigs[i+1]
			// only compare invigilations on the same calendar day
			if !sameCalendarDay(*inv1.Starttime, *inv2.Starttime) {
				continue
			}
			v.step("checking invigilator %d on %s", id, inv1.Starttime.Format("02.01."))

			realtime := inv1.Duration // TODO: calculate me

			// self- and reserve-invigilations occupy the whole slot: extend the
			// occupied time to the longest room duration at that start time.
			if inv1.IsSelfInvigilation || inv1.IsReserve {
				roomsAtStart, err := p.plannedRoomsAt(ctx, *inv1.Starttime)
				if err != nil {
					log.Error().Err(err).Time("start", *inv1.Starttime).Msg("cannot get rooms in slot")
				}
				for _, room := range roomsAtStart {
					if inv1.IsSelfInvigilation && (inv1.RoomName == nil || *inv1.RoomName != room.RoomName) {
						continue
					}
					if room.Duration > realtime {
						realtime = room.Duration
					}
				}
			}

			end1 := inv1.Starttime.Add(time.Duration(realtime) * time.Minute)

			if inv2.Starttime.Before(end1.Add(time.Duration(timelag) * time.Minute)) {
				comment := ""
				if inv1.IsReserve {
					comment = " (reserve first)"
				}

				v.errorf(ref{InvigilatorID: ptr(id), Starttime: inv1.Starttime},
					"Not enough time for invigilator %d on %s: invigilation ends %s and next begins %s: %g minutes between%s",
					id, inv1.Starttime.Format("02.01."), end1.Format("15:04"),
					inv2.Starttime.Format("15:04"),
					inv2.Starttime.Sub(end1).Minutes(), comment)
			}
		}
	}

	return v.finish(), nil
}
