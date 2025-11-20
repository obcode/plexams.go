package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/theckman/yacspin"
)

func (p *Plexams) ValidatePrePlannedExahmRooms() error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating pre-planned exahm rooms (booked and enough seats)")),
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

	exams := make([]*model.GeneratedExam, 0)
	generatedExams, err := p.dbClient.GetGeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).
			Msg("cannot get generated exams")
		return err
	}

	for _, exam := range generatedExams {
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil &&
			(exam.Constraints.RoomConstraints.Exahm || exam.Constraints.RoomConstraints.Seb) {
			exams = append(exams, exam)
		}
	}

	rooms, err := p.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).
			Msg("cannot get rooms")
	}
	roomsMap := make(map[string]*model.Room)
	for _, room := range rooms {
		roomsMap[room.Name] = room
	}

	for _, exam := range exams {
		prePlannedRooms, err := p.dbClient.PrePlannedRoomsForExam(ctx, exam.Ancode)
		if err != nil {
			log.Error().Err(err).
				Int("ancode", exam.Ancode).
				Msg("error while trying to get prePlannedRooms")
		}
		for _, prePlannedRoom := range prePlannedRooms {
			room := roomsMap[prePlannedRoom.RoomName]
			if exam.Constraints.RoomConstraints.Seb && !room.Seb {
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Room %s for %d. %s (%s) is not SEB-Room"),
					aurora.Magenta(room.Name), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer)))
			}

			if exam.Constraints.RoomConstraints.Exahm && !room.Exahm {
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Room %s for %d. %s (%s) is not EXaHM-Room"),
					aurora.Magenta(room.Name), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer)))
			}
		}

		// check if exam is planned in this slot if room is allowed
		planEntry, err := p.dbClient.PlanEntry(ctx, exam.Ancode)
		if err != nil {
			log.Error().Err(err).
				Int("ancode", exam.Ancode).
				Msg("cannot get plan entry for exam")
		}
		if planEntry == nil {
			validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Exam %d. %s (%s) has no plan entry yet"),
				aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer)))
		} else {
			roomsForSlot, err := p.RoomsForSlot(ctx, planEntry.DayNumber, planEntry.SlotNumber)
			if err != nil {
				log.Error().Err(err).
					Int("day", planEntry.DayNumber).
					Int("slot", planEntry.SlotNumber).
					Msg("cannot rooms for slot")
			}
			for _, prePlannedRoom := range prePlannedRooms {
				found := false
				for _, roomInSlot := range roomsForSlot.RoomNames {
					if prePlannedRoom.RoomName == roomInSlot {
						found = true
						break
					}
				}
				if !found {
					validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Room %s for Exam %d. %s (%s) in slot (%d/%d) is not allowed"),
						aurora.Magenta(prePlannedRoom.RoomName), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
						aurora.Blue(planEntry.DayNumber), aurora.Blue(planEntry.SlotNumber)))
				}
			}
		}

		// check if rooms have enough seats
		seats := 0
		for _, prePlannedRoom := range prePlannedRooms {
			room := roomsMap[prePlannedRoom.RoomName]
			seats += room.Seats
		}
		if seats < exam.StudentRegsCount {
			validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Not enough seats for Exam %d. %s (%s): %d seats planned, but %d students"),
				aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
				aurora.Cyan(seats), aurora.Cyan(exam.StudentRegsCount)))
		}
	}

	if len(validationMessages) > 0 {
		spinner.StopFailMessage(aurora.Sprintf(aurora.Red("%d problems found"), len(validationMessages)))
		err = spinner.StopFail()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
		for _, msg := range validationMessages {
			fmt.Printf("    ↪ %s\n", msg)
		}

	} else {
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}
