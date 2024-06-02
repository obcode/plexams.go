package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
)

func (p *Plexams) ValidateRoomsPerSlot() error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating rooms per slot (allowed and enough seats)")),
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

	slots := p.semesterConfig.Slots

	for _, slot := range slots {

		plannedExams, err := p.dbClient.GetExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).
				Int("day", slot.DayNumber).
				Int("time", slot.SlotNumber).
				Msg("error while getting exams planned in slot")
			return err
		}

		plannedRooms := make([]*model.PlannedRoom, 0)
		for _, plannedExam := range plannedExams {
			plannedRooms = append(plannedRooms, plannedExam.PlannedRooms...)
		}

		spinner.Message(aurora.Sprintf(aurora.Yellow("checking slot (%d/%d) with %d rooms in %d exams"),
			slot.DayNumber, slot.SlotNumber, len(plannedRooms), len(plannedExams)))

		allowedRooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).
				Int("day", slot.DayNumber).
				Int("time", slot.SlotNumber).
				Msg("error while getting allowed rooms for slot")
			return err
		}
		allAllowedRooms :=
			append(allowedRooms.NormalRooms,
				append(allowedRooms.LabRooms,
					append(allowedRooms.ExahmRooms,
						allowedRooms.NtaRooms...)...)...)

		for _, plannedRoom := range plannedRooms {
			if plannedRoom.RoomName == "ONLINE" {
				continue
			}

			if plannedRoom.RoomName == "No Room" {
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("No Room for %d students in slot (%d/%d)"),
					aurora.Magenta(len(plannedRoom.StudentsInRoom)), aurora.Blue(slot.DayNumber), aurora.Blue(slot.SlotNumber)))
				continue
			}

			isAllowed := false
			for _, allowedRoom := range allAllowedRooms {
				if allowedRoom.Name == plannedRoom.RoomName {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Room %s is not allowed in slot (%d/%d)"),
					aurora.Magenta(plannedRoom.RoomName), aurora.Blue(slot.DayNumber), aurora.Blue(slot.SlotNumber)))
			}
		}

		type roomSeats struct {
			seatsPlanned, seats int
		}
		seats := make(map[string]roomSeats)

		for _, plannedRoom := range plannedRooms {
			entry, ok := seats[plannedRoom.RoomName]

			// TODO: Remove this hack
			if strings.HasPrefix(plannedRoom.RoomName, "ONLINE") {
				continue
			}

			if !ok {
				seats[plannedRoom.RoomName] = roomSeats{seatsPlanned: len(plannedRoom.StudentsInRoom), seats: p.GetRoomInfo(plannedRoom.RoomName).Seats}
			} else {
				seats[plannedRoom.RoomName] = roomSeats{seatsPlanned: len(plannedRoom.StudentsInRoom) + entry.seatsPlanned, seats: p.GetRoomInfo(plannedRoom.RoomName).Seats}
			}
		}

		for roomName, roomSeats := range seats {
			if roomSeats.seatsPlanned > roomSeats.seats {
				validationMessages = append(validationMessages,
					aurora.Sprintf(aurora.Red("Room %s is overbooked in slot (%d/%d): %d seats planned, but only %d available"),
						aurora.Magenta(roomName), aurora.Blue(slot.DayNumber), aurora.Blue(slot.SlotNumber),
						aurora.Cyan(roomSeats.seatsPlanned), aurora.Cyan(roomSeats.seats)))
			}
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
func (p *Plexams) ValidateRoomsNeedRequest() error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating rooms which needs requests")),
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

	roomTimetables, err := p.GetReservations()
	if err != nil {
		log.Error().Err(err).Msg("cannot get reservations")
		return err
	}

	bookedEntries, err := p.ExahmRoomsFromBooked()
	if err != nil {
		log.Error().Err(err).Msg("cannot get booked entries")
		return err
	}

	for _, bookedEntry := range bookedEntries {
		for _, roomName := range bookedEntry.Rooms {
			timeRanges, ok := roomTimetables[roomName]
			if !ok {
				timeRanges = make([]TimeRange, 0, 1)
			}
			roomTimetables[roomName] = append(timeRanges, TimeRange{
				From:     bookedEntry.From,
				Until:    bookedEntry.Until,
				Approved: bookedEntry.Approved,
			})
		}
	}

	log.Debug().Interface("timetables", roomTimetables).Msg("found booked and reserved rooms")

	plannedRooms, err := p.PlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all planned rooms")
	}

	reservationFound := 0

PLANNEDROOM:
	for _, plannedRoom := range plannedRooms {

		if !p.roomInfo[plannedRoom.RoomName].NeedsRequest {
			log.Debug().Str("room", plannedRoom.RoomName).Msg("room needs no request")
			continue
		}

		spinner.Message(aurora.Sprintf(aurora.Yellow("checking room  %s in slot (%d/%d)"),
			plannedRoom.RoomName, plannedRoom.Day, plannedRoom.Slot))

		startTime := p.getSlotTime(plannedRoom.Day, plannedRoom.Slot).Local()
		endTime := startTime.Add(time.Duration(plannedRoom.Duration) * time.Minute)

		for _, timerange := range roomTimetables[plannedRoom.RoomName] {
			if timerange.From.Before(startTime) && timerange.Until.After(endTime) {
				log.Debug().Str("room", plannedRoom.RoomName).Msg("found reservation")
				reservationFound++

				if !timerange.Approved {
					validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Reservation for room %s found in slot (%d/%d) is not yet approved"),
						aurora.Magenta(plannedRoom.RoomName), aurora.Blue(plannedRoom.Day), aurora.Blue(plannedRoom.Slot)))
				}

				continue PLANNEDROOM
			}
		}

		validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("No Reservation for room %s found in slot (%d/%d)"),
			aurora.Magenta(plannedRoom.RoomName), aurora.Blue(plannedRoom.Day), aurora.Blue(plannedRoom.Slot)))
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
		spinner.StopMessage(aurora.Sprintf(aurora.Green("found %d reservations"), reservationFound))
		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return nil
}

func (p *Plexams) ValidateRoomsPerExam() error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating rooms per exam")),
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

	exams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams in plan")
	}

	for _, exam := range exams {
		if exam.PlanEntry == nil {
			continue
		}

		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}

		spinner.Message(aurora.Sprintf(aurora.Yellow("checking rooms for %d. %s (%s) with %d students and %d ntas"),
			exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer, exam.StudentRegsCount, len(exam.Ntas)))
		// check if each student has a room
		allStudentRegs := make([]*model.StudentReg, 0)
		for _, primussExam := range exam.PrimussExams {
			allStudentRegs = append(allStudentRegs, primussExam.StudentRegs...)
		}

		// rooms, err := p.dbClient.RoomsForAncode(ctx, exam.Exam.Ancode)
		// if err != nil {
		// 	log.Error().Err(err).Int("ancode", exam.Exam.Ancode).Msg("cannot get rooms for ancode")
		// 	return err
		// }

		allStudentsInRooms := make([]string, 0)
		for _, room := range exam.PlannedRooms {
			if room.RoomName != "No Room" {
				allStudentsInRooms = append(allStudentsInRooms, room.StudentsInRoom...)
			}
		}

		for _, studentReg := range allStudentRegs {
			studentHasSeat := false
			for _, mtknr := range allStudentsInRooms {
				if studentReg.Mtknr == mtknr {
					studentHasSeat = true
					break
				}
			}
			if !studentHasSeat {
				validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Student %s (%s) has no seat for exam %d. %s (%s) in slot (%d,%d)"),
					aurora.Magenta(studentReg.Name), aurora.Magenta(studentReg.Mtknr), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
					aurora.BrightBlue(exam.PlanEntry.DayNumber), aurora.BrightBlue(exam.PlanEntry.SlotNumber)))
			}
		}

		// check if room constraints of exams are met
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil {
			if exam.Constraints.RoomConstraints.ExahmRooms {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).Exahm {
						validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Is not an exahm room %s for exam %d. %s (%s) in slot (%d,%d)"),
							aurora.Magenta(room.RoomName), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
							aurora.BrightBlue(exam.PlanEntry.DayNumber), aurora.BrightBlue(exam.PlanEntry.SlotNumber)))
					}
				}
			}
			if exam.Constraints.RoomConstraints.Seb {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).Seb {
						validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Is not an exahm room %s for exam %d. %s (%s) in slot (%d,%d)"),
							aurora.Magenta(room.RoomName), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
							aurora.BrightBlue(exam.PlanEntry.DayNumber), aurora.BrightBlue(exam.PlanEntry.SlotNumber)))
					}
				}
			}
			if exam.Constraints.RoomConstraints.Lab {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).Lab {
						validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Is not a lab %s for exam %d. %s (%s) in slot (%d,%d)"),
							aurora.Magenta(room.RoomName), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
							aurora.BrightBlue(exam.PlanEntry.DayNumber), aurora.BrightBlue(exam.PlanEntry.SlotNumber)))
					}
				}
			}
			if exam.Constraints.RoomConstraints.PlacesWithSocket {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).PlacesWithSocket && !p.GetRoomInfo(room.RoomName).Lab {
						validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("Is not a room with places with sockets %s for exam %d. %s (%s) in slot (%d,%d)"),
							aurora.Magenta(room.RoomName), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
							aurora.BrightBlue(exam.PlanEntry.DayNumber), aurora.BrightBlue(exam.PlanEntry.SlotNumber)))
					}
				}
			}
		}

		// check rooms for NTAs
		// - needsRoomAlone okay
		if len(exam.Ntas) > 0 {
			spinner.Message(aurora.Sprintf(aurora.Yellow("checking rooms for ntas")))
			for _, nta := range exam.Ntas {
				if nta.NeedsRoomAlone {
					var roomForNta *model.PlannedRoom
					for _, room := range exam.PlannedRooms {
						if room.NtaMtknr != nil && *room.NtaMtknr == nta.Mtknr {
							roomForNta = room
							break
						}
					}
				OUTER:
					for _, room := range exam.PlannedRooms {
						if room.RoomName == roomForNta.RoomName {
							for _, mtknr := range room.StudentsInRoom {
								if mtknr != nta.Mtknr {
									validationMessages = append(validationMessages, aurora.Sprintf(aurora.Red("NTA %s has room %s not alone for exam %d. %s (%s) in slot (%d,%d)"),
										aurora.Magenta(nta.Name), aurora.Magenta(room.RoomName), aurora.Cyan(exam.Ancode), aurora.Cyan(exam.ZpaExam.Module), aurora.Cyan(exam.ZpaExam.MainExamer),
										aurora.BrightBlue(exam.PlanEntry.DayNumber), aurora.BrightBlue(exam.PlanEntry.SlotNumber)))
									break OUTER
								}
							}
						}
					}
				}
			}
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

func (p *Plexams) ValidateRoomsTimeDistance() error {
	ctx := context.Background()
	timelag := viper.GetInt("rooms.timelag")

	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" validating time lag of planned rooms (%d minutes)"), timelag),
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

	for _, day := range p.semesterConfig.Days {
		spinner.Message(aurora.Sprintf(aurora.Yellow("checking day %d (%s)"), day.Number, day.Date.Format("02.01.06")))

		for i := range p.semesterConfig.Starttimes {
			if i == len(p.semesterConfig.Days)-1 {
				continue
			}
			slot1, slot2 := i+1, i+2
			log.Debug().Int("slot 1", slot1).Int("slot 2", slot2).Msg("checking slot")

			plannedRoomsSlot1, err := p.dbClient.PlannedRoomsInSlot(ctx, day.Number, slot1)
			if err != nil {
				log.Error().Err(err).
					Int("day", day.Number).
					Int("time", slot1).
					Msg("error while getting rooms planned in slot")
				return err
			}

			rooms1 := make(map[string]int)
			for _, room := range plannedRoomsSlot1 {
				duration, ok := rooms1[room.RoomName]
				if !ok {
					rooms1[room.RoomName] = room.Duration
				} else {
					if duration < room.Duration {
						rooms1[room.RoomName] = room.Duration
					}
				}
			}

			plannedRoomsSlot2, err := p.dbClient.PlannedRoomsInSlot(ctx, day.Number, slot2)
			if err != nil {
				log.Error().Err(err).
					Int("day", day.Number).
					Int("time", slot2).
					Msg("error while getting rooms planned in slot")
				return err
			}

			rooms2 := set.NewSet[string]()
			for _, room := range plannedRoomsSlot2 {
				rooms2.Add(room.RoomName)
			}

			for roomName, maxDuration := range rooms1 {
				if rooms2.Contains(roomName) {
					start, err := time.Parse("15:04", p.semesterConfig.Starttimes[i].Start)
					if err != nil {
						log.Error().Err(err).Str("starttime", p.semesterConfig.Starttimes[i].Start).Msg("cannot parse starttime")
						return err
					}
					endSlot1 := start.Add(time.Duration(maxDuration) * time.Minute)

					startSlot2, err := time.Parse("15:04", p.semesterConfig.Starttimes[i+1].Start)
					if err != nil {
						log.Error().Err(err).Str("starttime", p.semesterConfig.Starttimes[i].Start).Msg("cannot parse starttime")
						return err
					}
					log.Debug().Str("room", roomName).Int("max duration", maxDuration).
						Str("starttime slot 1", p.semesterConfig.Starttimes[i].Start).
						Str("endtime slot 1", endSlot1.Format("15:04")).
						Str("starttime slot 2", startSlot2.Format("15:04")).
						Msg("checking")

					diff := time.Duration(timelag) * time.Minute

					if startSlot2.Before(endSlot1.Add(diff)) {
						validationMessages = append(validationMessages, aurora.Sprintf(
							"Not enough time in room %s between slot (%d/%d) ends %s and slot (%d/%d) begins %s: %g minutes between",
							aurora.Magenta(roomName), aurora.BrightBlue(day.Number), aurora.BrightBlue(slot1), aurora.Magenta(endSlot1.Format("15:04")),
							aurora.BrightBlue(day.Number), aurora.BrightBlue(slot2), aurora.Magenta(startSlot2.Format("15:04")),
							aurora.Magenta(startSlot2.Sub(endSlot1).Minutes()),
						))
					}
				}
			}
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
