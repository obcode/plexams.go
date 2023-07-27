package plexams

import (
	"context"
	"fmt"
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gookit/color"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateInvigilatorRequirements() error {
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.
		Printf(" ---   validating invigilator requirements  --- \n")

	ctx := context.Background()
	invigilationTodos, err := p.InvigilationTodos(ctx)
	if err != nil {
		return err
	}

	for _, invigilator := range invigilationTodos.Invigilators {
		log.Debug().Str("name", invigilator.Teacher.Shortname).Msg("checking constraints")

		// days ok
		for _, invigilationDay := range invigilator.Todos.InvigilationDays {
			for _, excludedDay := range invigilator.Requirements.ExcludedDays {
				if invigilationDay == excludedDay {
					color.Red.Printf("%s has invigilation on excluded day %d\n", invigilator.Teacher.Shortname, invigilationDay)
				}
			}
		}

		// onlySlotsOk
		if invigilator.Requirements.OnlyInSlots != nil && len(invigilator.Requirements.OnlyInSlots) > 0 {
			for _, invigilation := range invigilator.Todos.Invigilations {
				slotOk := false
				for _, slot := range invigilator.Requirements.OnlyInSlots {
					if invigilation.Slot.DayNumber == slot.DayNumber && invigilation.Slot.SlotNumber == slot.SlotNumber {
						slotOk = true
						break
					}
				}
				if !slotOk {
					color.Red.Printf("%s has invigilation in not allowed slot [%d,%d]\n", invigilator.Teacher.Shortname,
						invigilation.Slot.DayNumber, invigilation.Slot.SlotNumber)
				}
			}
		}

		// nur ein Raum oder Reserve
		invigilationSlots := set.NewSet[int]() // day * 10 + slot
		for _, invigilation := range invigilator.Todos.Invigilations {
			combinedNumber := invigilation.Slot.DayNumber*10 + invigilation.Slot.SlotNumber
			if invigilationSlots.Contains(combinedNumber) {
				color.Red.Printf("%s has more than one invigilation in slot (%d,%d)\n", invigilator.Teacher.Shortname,
					invigilation.Slot.DayNumber, invigilation.Slot.SlotNumber)
			}
			invigilationSlots.Add(combinedNumber)

		}

		// wenn gleichzeitig Prüfung, dann nur self-invigilation
		exams, err := p.dbClient.PlannedExamsByMainExamer(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("name", invigilator.Teacher.Shortname).Msg("cannot get exams")
		}

		for _, exam := range exams {
			for _, invigilation := range invigilator.Todos.Invigilations {
				if exam.Slot.DayNumber == invigilation.Slot.DayNumber &&
					exam.Slot.SlotNumber == invigilation.Slot.SlotNumber {
					if invigilation.IsReserve {
						color.Red.Printf("%s has reserve invigilation during own exam %d. %s in slot (%d,%d)\n", invigilator.Teacher.Shortname,
							exam.Constraints.Ancode, exam.Exam.ZpaExam.Module, exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}

					roomsForExam, err := p.dbClient.RoomsForAncode(ctx, exam.Exam.Ancode)
					rooms := set.NewSet[string]()
					for _, room := range roomsForExam {
						rooms.Add(room.RoomName)
					}

					if err != nil {
						log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("cannot get rooms for exam")
					} else {
						if rooms.Cardinality() > 1 {
							color.Red.Printf("%s has invigilation during own exam with more than one room: %d. %s in slot (%d,%d): found rooms %v\n", invigilator.Teacher.Shortname,
								exam.Constraints.Ancode, exam.Exam.ZpaExam.Module, exam.Slot.DayNumber, exam.Slot.SlotNumber, rooms)
						}
					}

				}
			}
		}

	}

	return nil
}

// TODO: NTA- und Reserve-Aufsicht (wenn NTA) nicht im folgenden Slot einteilen!
func (p *Plexams) ValidateInvigilatorSlots() error {
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.
		Printf(" ---   validating invigilator for all slots  --- \n")

	ctx := context.Background()
	// count rooms and reserves without and print number
	roomWithoutInvigilatorDay := make(map[int]int)
	slotWithoutReserveDay := make(map[int]int)

	// all rooms and reserve max one invigilator
	for _, slot := range p.semesterConfig.Slots {
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
		} else if len(invigilations) > 1 {
			color.Red.Printf("more than one reserve invigilator in slot (%d,%d): ", slot.DayNumber, slot.SlotNumber)
			for _, invigilation := range invigilations {
				color.Red.Printf("%d, ", invigilation.InvigilatorID)
			}
		}

		for _, room := range rooms {
			if room == "No Room" {
				continue
			}
			invigilations, err := p.dbClient.GetInvigilationInSlot(ctx, room, slot.DayNumber, slot.SlotNumber)
			if err != nil {
				log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Str("room", room).
					Msg("cannot get reserve invigilator")
			}
			if len(invigilations) == 0 {
				roomWithoutInvigilatorDay[slot.DayNumber]++
			} else if len(invigilations) > 1 {
				color.Red.Printf("more than one invigilator for room %s in slot (%d,%d): ", room,
					slot.DayNumber, slot.SlotNumber)
				for _, invigilation := range invigilations {
					color.Red.Printf("%d, ", invigilation.InvigilatorID)
				}
				fmt.Println()
			}
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
				color.Red.Printf("Day %2d: %2d open invigilations, ", day, roomsWithoutInvig+slotsWithoutReserve)
				if roomsWithoutInvig > 0 {
					color.Red.Printf("%2d rooms without invigilator", roomsWithoutInvig)
				} else {
					fmt.Print("                            ")
				}
				if slotsWithoutReserve > 0 {
					color.Red.Printf(", %2d slots without reserve", slotsWithoutReserve)
				}
				fmt.Println()
			}
		}
	}

	return nil
}
