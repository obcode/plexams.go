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
	v := newValidation(p.TimeForSlot, reporter, "invigilator-requirements", "validating invigilator requirements")

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
					v.errorf(ref{InvigilatorID: ptr(invigilator.Teacher.ID), Day: ptr(invigilationDay)},
						"%s has invigilation on excluded day %d", invigilator.Teacher.Fullname, invigilationDay)
				}
			}
		}

		// nur ein Raum oder Reserve
		invigilationSlots := set.NewSet[int]() // day * 10 + slot
		for _, invigilation := range invigilator.Todos.Invigilations {
			combinedNumber := invigilation.Slot.DayNumber*10 + invigilation.Slot.SlotNumber
			if invigilationSlots.Contains(combinedNumber) {
				v.errorf(ref{InvigilatorID: ptr(invigilator.Teacher.ID), Day: ptr(invigilation.Slot.DayNumber), Slot: ptr(invigilation.Slot.SlotNumber)},
					"%s has more than one invigilation in slot (%d,%d)",
					invigilator.Teacher.Fullname, invigilation.Slot.DayNumber, invigilation.Slot.SlotNumber)
			}
			invigilationSlots.Add(combinedNumber)

		}

		// wenn gleichzeitig Prüfung, dann nur self-invigilation
		exams, err := p.PlannedExamsByExamer(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("name", invigilator.Teacher.Shortname).Msg("cannot get exams")
		}

		for _, exam := range exams {
			for _, invigilation := range invigilator.Todos.Invigilations {
				if exam.PlanEntry.DayNumber == invigilation.Slot.DayNumber &&
					exam.PlanEntry.SlotNumber == invigilation.Slot.SlotNumber {
					if invigilation.IsReserve {
						v.errorf(ref{Ancode: ptr(exam.Constraints.Ancode), InvigilatorID: ptr(invigilator.Teacher.ID), Day: ptr(invigilation.Slot.DayNumber), Slot: ptr(invigilation.Slot.SlotNumber)},
							"%s has reserve invigilation during own exam %d. %s in slot (%d,%d)",
							invigilator.Teacher.Fullname, exam.Constraints.Ancode, exam.ZpaExam.Module,
							invigilation.Slot.DayNumber, invigilation.Slot.SlotNumber)
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
							v.errorf(ref{Ancode: ptr(exam.Constraints.Ancode), InvigilatorID: ptr(invigilator.Teacher.ID), Day: ptr(invigilation.Slot.DayNumber), Slot: ptr(invigilation.Slot.SlotNumber)},
								"%s has invigilation during own exam with more than one room: %d. %s in slot (%d,%d): found rooms %v",
								invigilator.Teacher.Fullname, exam.Constraints.Ancode, exam.ZpaExam.Module,
								invigilation.Slot.DayNumber, invigilation.Slot.SlotNumber, rooms)
						}
					}

				}
			}
		}

	}

	return v.finish(), nil
}
func (p *Plexams) ValidateInvigilationDups(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(p.TimeForSlot, reporter, "invigilation-duplicates", "validating invigilator duplicates")

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
		room string
		day  int
		slot int
	}

	invigilationsMap := make(map[key]*model.Invigilation)

	v.step("checking %d invigilations", len(invigilations))
	for _, invigilation := range invigilations {
		var room string
		if invigilation.RoomName == nil {
			room = "null"
		} else {
			room = *invigilation.RoomName
		}
		key := key{
			room: room,
			day:  invigilation.Slot.DayNumber,
			slot: invigilation.Slot.SlotNumber,
		}

		_, ok := invigilationsMap[key]
		if ok {
			v.errorf(ref{Room: invigilation.RoomName, Day: ptr(invigilation.Slot.DayNumber), Slot: ptr(invigilation.Slot.SlotNumber), InvigilatorID: ptr(invigilation.InvigilatorID)},
				"double entry for {roomname: %s, slot.daynumber: %d, slot.slotnumber: %d}",
				room, invigilation.Slot.DayNumber, invigilation.Slot.SlotNumber)
		} else {
			invigilationsMap[key] = invigilation
		}
	}

	return v.finish(), nil
}

// TODO: NTA- und Reserve-Aufsicht (wenn NTA) nicht im folgenden Slot einteilen!
func (p *Plexams) ValidateInvigilatorSlots(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(p.TimeForSlot, reporter, "invigilator-slots", "validating invigilator for all slots")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	// count rooms and reserves without and print number
	roomWithoutInvigilatorDay := make(map[int]int)
	slotWithoutReserveDay := make(map[int]int)

	maxInvigsMissingInOneSlot := make(map[int]int)

	// all rooms and reserve max one invigilator
	for _, slot := range p.semesterConfig.Slots {
		invigsMissing := 0
		v.step("checking slot (%d,%d)", slot.DayNumber, slot.SlotNumber)

		rooms, err := p.PlannedRoomNamesInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Msg("cannot get rooms for")
		}
		if len(rooms) == 0 {
			continue
		}
		invigilations, err := p.dbClient.GetInvigilationInSlot(ctx, "reserve", slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Msg("cannot get reserve invigilator")
		}

		if len(invigilations) == 0 {
			slotWithoutReserveDay[slot.DayNumber]++
			invigsMissing++
		} else if len(invigilations) > 1 {
			v.errorf(ref{Day: ptr(slot.DayNumber), Slot: ptr(slot.SlotNumber)},
				"more than one reserve invigilator in slot (%d,%d)", slot.DayNumber, slot.SlotNumber)
		}

		for _, room := range rooms {
			invigilations, err := p.dbClient.GetInvigilationInSlot(ctx, room, slot.DayNumber, slot.SlotNumber)
			if err != nil {
				log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Str("room", room).
					Msg("cannot get reserve invigilator")
			}
			if len(invigilations) == 0 {
				roomWithoutInvigilatorDay[slot.DayNumber]++
				invigsMissing++
			} else if len(invigilations) > 1 {
				v.warnf(ref{Room: ptr(room), Day: ptr(slot.DayNumber), Slot: ptr(slot.SlotNumber)},
					"more than one invigilator for room %s in slot (%d,%d)", room, slot.DayNumber, slot.SlotNumber)
			}
		}
		if invigsMissing > maxInvigsMissingInOneSlot[slot.DayNumber] {
			maxInvigsMissingInOneSlot[slot.DayNumber] = invigsMissing
		}
	}

	if len(roomWithoutInvigilatorDay) > 0 || len(slotWithoutReserveDay) > 0 {
		keySet := set.NewSet[int]()
		for k := range roomWithoutInvigilatorDay {
			keySet.Add(k)
		}
		for k := range slotWithoutReserveDay {
			keySet.Add(k)
		}
		keys := keySet.ToSlice()

		sort.Ints(keys)

		for _, day := range keys {
			roomsWithoutInvig := roomWithoutInvigilatorDay[day]
			slotsWithoutReserve := slotWithoutReserveDay[day]

			if roomsWithoutInvig+slotsWithoutReserve > 0 {
				v.warnf(ref{Day: ptr(day)},
					"Day %d: %d open invigilations (%d max. in one slot), %d rooms without invigilator, %d slots without reserve",
					day, roomsWithoutInvig+slotsWithoutReserve, maxInvigsMissingInOneSlot[day], roomsWithoutInvig, slotsWithoutReserve)
			}
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateInvigilationsTimeDistance(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	timelag := p.generationTimelagMin(ctx)

	v := newValidation(p.TimeForSlot, reporter, "invigilations-time-distance",
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

	type slot struct {
		day  int
		slot int
	}

	invigilations := make(map[slot][]*model.Invigilation)
	for _, invigilation := range allInvigilations {
		slot := slot{
			day:  invigilation.Slot.DayNumber,
			slot: invigilation.Slot.SlotNumber,
		}

		invigilationsInSlot, ok := invigilations[slot]
		if !ok {
			invigilationsInSlot = make([]*model.Invigilation, 0, 1)
		}
		invigilations[slot] = append(invigilationsInSlot, invigilation)
	}

	for _, day := range p.semesterConfig.Days {
		v.step("checking day %d (%s)", day.Number, day.Date.Format("02.01.06"))

		for i := range p.semesterConfig.Starttimes {
			if i == len(p.semesterConfig.Days)-1 {
				continue
			}
			slot1, slot2 := slot{day: day.Number, slot: i + 1}, slot{day: day.Number, slot: i + 2}
			log.Debug().Interface("slot 1", slot1).Interface("slot 2", slot2).Msg("checking slot")

			invigilationsSlot1, ok := invigilations[slot1]
			if !ok || len(invigilationsSlot1) == 0 {
				log.Debug().Interface("slot", slot1).Msg("no invigilations in slot")
				continue
			}

			invigilationsSlot2, ok := invigilations[slot2]
			if !ok || len(invigilationsSlot2) == 0 {
				log.Debug().Interface("slot", slot2).Msg("no invigilations in slot")
				continue
			}

			for _, invigilation1 := range invigilationsSlot1 {
				for _, invigilation2 := range invigilationsSlot2 {
					if invigilation1.InvigilatorID != invigilation2.InvigilatorID {
						continue
					}

					startSlot1 := p.getSlotTime(invigilation1.Slot.DayNumber, invigilation1.Slot.SlotNumber)
					startSlot2 := p.getSlotTime(invigilation2.Slot.DayNumber, invigilation2.Slot.SlotNumber)

					realtime := invigilation1.Duration // TODO: calculate me

					if invigilation1.IsSelfInvigilation {
						roomsInSlot, err := p.dbClient.PlannedRoomsInSlot(ctx, slot1.day, slot1.slot)
						if err != nil {
							log.Error().Err(err).Interface("slot", slot1).Msg("cannot get rooms in slot")
						}
						for _, room := range roomsInSlot {
							if invigilation1.RoomName == &room.RoomName {
								if room.Duration > realtime {
									realtime = room.Duration
								}
							}
						}
					}

					if invigilation1.IsReserve {
						roomsInSlot, err := p.dbClient.PlannedRoomsInSlot(ctx, slot1.day, slot1.slot)
						if err != nil {
							log.Error().Err(err).Interface("slot", slot1).Msg("cannot get rooms in slot")
						}
						for _, room := range roomsInSlot {
							if room.Duration > realtime {
								realtime = room.Duration
							}
						}
					}

					endSlot1 := startSlot1.Add(time.Duration(realtime) * time.Minute)

					if startSlot2.Before(endSlot1.Add(time.Duration(timelag) * time.Minute)) {
						comment := ""
						if invigilation1.IsReserve {
							comment = " (reserve in first slot)"
						}

						v.errorf(ref{InvigilatorID: ptr(invigilation1.InvigilatorID), Day: ptr(day.Number), Slot: ptr(slot2.slot)},
							"Not enough time for invigilator %d between slot (%d/%d) ends %s and slot (%d/%d) begins %s: %g minutes between%s",
							invigilation1.InvigilatorID, day.Number, slot1.slot, endSlot1.Format("15:04"),
							day.Number, slot2.slot, startSlot2.Format("15:04"),
							startSlot2.Sub(endSlot1).Minutes(), comment)
					}
				}
			}
		}
	}

	return v.finish(), nil
}
