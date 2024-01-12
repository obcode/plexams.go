package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) ValidateInvigilatorRequirements() error {

	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating invigilator requirements")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make([]string, 0)

	ctx := context.Background()
	spinner.Message(aurora.Sprintf(aurora.Magenta("recalculating todos")))
	invigilationTodos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		return err
	}

	for _, invigilator := range invigilationTodos.Invigilators {
		spinner.Message(aurora.Sprintf(aurora.Cyan("checking %s"), aurora.Magenta(invigilator.Teacher.Fullname)))
		log.Debug().Str("name", invigilator.Teacher.Shortname).Msg("checking constraints")

		// days ok
		for _, invigilationDay := range invigilator.Todos.InvigilationDays {
			for _, excludedDay := range invigilator.Requirements.ExcludedDays {
				if invigilationDay == excludedDay {
					validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("%s has invigilation on excluded day %d"),
						aurora.Magenta(invigilator.Teacher.Fullname), aurora.Cyan(invigilationDay)))
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
					validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("%s has invigilation not allowed slot (%d,%d)"),
						aurora.Magenta(invigilator.Teacher.Fullname), aurora.Cyan(invigilation.Slot.DayNumber), aurora.Cyan(invigilation.Slot.SlotNumber)))
				}
			}
		}

		// nur ein Raum oder Reserve
		invigilationSlots := set.NewSet[int]() // day * 10 + slot
		for _, invigilation := range invigilator.Todos.Invigilations {
			combinedNumber := invigilation.Slot.DayNumber*10 + invigilation.Slot.SlotNumber
			if invigilationSlots.Contains(combinedNumber) {
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("%s has more than one invigilation in slot (%d,%d)"),
					aurora.Magenta(invigilator.Teacher.Fullname), aurora.Cyan(invigilation.Slot.DayNumber), aurora.Cyan(invigilation.Slot.SlotNumber)))
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
						validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("%s has reserve invigilation during own exam %d. %s in slot (%d,%d)"),
							aurora.Magenta(invigilator.Teacher.Fullname), aurora.Cyan(exam.Constraints.Ancode), aurora.Cyan(exam.Exam.ZpaExam.Module),
							aurora.Cyan(invigilation.Slot.DayNumber), aurora.Cyan(invigilation.Slot.SlotNumber)))
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
							validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("%s has invigilation during own exam with more than one room: %d. %s in slot (%d,%d): found rooms %v"),
								aurora.Magenta(invigilator.Teacher.Fullname), aurora.Cyan(exam.Constraints.Ancode), aurora.Cyan(exam.Exam.ZpaExam.Module),
								aurora.Cyan(invigilation.Slot.DayNumber), aurora.Cyan(invigilation.Slot.SlotNumber), aurora.Cyan(rooms)))
						}
					}

				}
			}
		}

	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems found"),
			len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("no problems found")))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}
func (p *Plexams) ValidateInvigilationDups() error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating invigilator duplicates")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make([]string, 0)

	ctx := context.Background()
	spinner.Message(aurora.Sprintf(aurora.Magenta("getting all invigilations")))
	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all invigilations")
		return err
	}

	type key struct {
		room string
		day  int
		slot int
	}

	invigilationsMap := make(map[key]*model.Invigilation)

	spinner.Message(aurora.Sprintf(aurora.Cyan("checking %d invigilations"), aurora.Magenta(len(invigilations))))
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
			var roomName string
			if invigilation.RoomName == nil {
				roomName = "null"
			} else {
				roomName = fmt.Sprintf("\"%s\"", *invigilation.RoomName)
			}
			validationMessages = append(validationMessages,
				aurora.Sprintf(aurora.Red("double entry for {roomname: %s, \"slot.daynumber\": %d, \"slot.slotnumber\": %d}"),
					aurora.Magenta(roomName), aurora.Cyan(invigilation.Slot.DayNumber), aurora.Cyan(invigilation.Slot.SlotNumber)))
		} else {
			invigilationsMap[key] = invigilation
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d invigilations, %d problems found"),
			len(invigilations), len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("%d invigilations, no problems found"), len(invigilations)))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}

// TODO: NTA- und Reserve-Aufsicht (wenn NTA) nicht im folgenden Slot einteilen!
func (p *Plexams) ValidateInvigilatorSlots() error {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating invigilator for all slots")),
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailMessage:   "error",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		log.Debug().Err(err).Msg("cannot create spinner")
	}
	err = spinner.Start()
	if err != nil {
		log.Debug().Err(err).Msg("cannot start spinner")
	}

	validationMessages := make([]string, 0)

	ctx := context.Background()

	// count rooms and reserves without and print number
	roomWithoutInvigilatorDay := make(map[int]int)
	slotWithoutReserveDay := make(map[int]int)

	// all rooms and reserve max one invigilator
	for _, slot := range p.semesterConfig.Slots {
		spinner.Message(aurora.Sprintf(aurora.Magenta("checking slot (%d,%d)"),
			aurora.Cyan(slot.DayNumber), aurora.Cyan(slot.SlotNumber)))

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
			validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("more than one reserve invigilator in slot (%d,%d)"),
				aurora.Magenta(slot.DayNumber), aurora.Magenta(slot.SlotNumber)))
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
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("more than one invigilator for room %s in slot (%d,%d)"),
					aurora.Magenta(room), aurora.Magenta(slot.DayNumber), aurora.Magenta(slot.SlotNumber)))
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
				var msg strings.Builder
				msg.WriteString(aurora.Sprintf(aurora.Red("Day %2d: %2d open invigilations, "),
					aurora.Magenta(day), aurora.Cyan(roomsWithoutInvig+slotsWithoutReserve)))

				if roomsWithoutInvig > 0 {
					msg.WriteString(aurora.Sprintf(aurora.Red("%2d rooms without invigilator,"), aurora.Cyan(roomsWithoutInvig)))
				} else {
					msg.WriteString("                             ")
				}
				if slotsWithoutReserve > 0 {
					msg.WriteString(aurora.Sprintf(aurora.Red("%2d slots without reserve"), aurora.Cyan(slotsWithoutReserve)))
				}

				validationMessages = append(validationMessages, msg.String())
			}
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems found"),
			len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("%s\n", msg)
		}

	} else {
		spinner.StopMessage(aurora.Sprintf(aurora.Green("no problems found")))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}
