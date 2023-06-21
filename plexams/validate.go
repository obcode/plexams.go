package plexams

import (
	"context"
	"math"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// TODO: Validate if all NTAs have MTKNR

var count = 0

func (p *Plexams) ValidateConflicts(onlyPlannedByMe bool, ancode int) error {
	count = 0
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating conflicts   --- ")

	planAncodeEntries, err := p.dbClient.PlanAncodeEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return err
	}

	planAncodeEntriesNotPlannedByMe := set.NewSet[int]()
	for _, entry := range planAncodeEntries {
		constraints, err := p.dbClient.GetConstraintsForAncode(ctx, entry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", entry.Ancode).Msg("cannot get constraints for ancode")
			return err
		}
		if constraints != nil && constraints.NotPlannedByMe {
			planAncodeEntriesNotPlannedByMe.Add(entry.Ancode)
		}
	}

	studentRegs, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return err
	}

	for _, studentReg := range studentRegs {
		validateStudentReg(studentReg, planAncodeEntries, planAncodeEntriesNotPlannedByMe, onlyPlannedByMe, ancode)
	}
	return nil
}

func validateStudentReg(studentReg *model.StudentRegsPerStudent, planAncodeEntries []*model.PlanAncodeEntry,
	planAncodeEntriesNotPlannedByMe set.Set[int], onlyPlannedByMe bool, ancode int) {
	log.Debug().Str("name", studentReg.Student.Name).Str("mtknr", studentReg.Student.Mtknr).Msg("checking regs for student")

	planAncodeEntriesForStudent := make([]*model.PlanAncodeEntry, 0)
	for _, ancode := range studentReg.Ancodes {
		for _, planEntry := range planAncodeEntries {
			if ancode == planEntry.Ancode {
				planAncodeEntriesForStudent = append(planAncodeEntriesForStudent, planEntry)
			}
		}
	}

	if len(planAncodeEntriesForStudent) == 0 {
		log.Debug().Str("name", studentReg.Student.Name).Str("mtknr", studentReg.Student.Mtknr).Msg("no exam for student in plan")
		return
	}

	p := planAncodeEntriesForStudent
	for i := 0; i < len(planAncodeEntriesForStudent); i++ {
		for j := i + 1; j < len(planAncodeEntriesForStudent); j++ {
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber &&
				p[i].Ancode == p[j].Ancode {
				continue
			}
			if onlyPlannedByMe &&
				planAncodeEntriesNotPlannedByMe.Contains(p[i].Ancode) &&
				planAncodeEntriesNotPlannedByMe.Contains(p[j].Ancode) {
				log.Debug().Int("ancode1", p[i].Ancode).Int("ancode2", p[j].Ancode).
					Msg("both ancodes not planned by me")
				continue
			}
			if ancode != 0 && p[i].Ancode != ancode && p[j].Ancode != ancode {
				continue
			}

			// same slot
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber {
				count++
				color.Red.Printf("%3d. Same slot: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n", count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				count++
				color.Red.Printf("%3d. Adjacent slots: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n", count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				count++
				color.Yellow.Printf("%3d. Same day: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n", count,
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			}
		}
	}
}

func (p *Plexams) ValidateConstraints() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating constraints   --- ")

	constraints, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
	}

	for _, constraint := range constraints {
		slot, err := p.SlotForAncode(ctx, constraint.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", constraint.Ancode).Msg("cannot get slot for ancode")
		}

		if slot == nil {
			continue
		}

		if constraint.FixedDay != nil {
			color.Red.Println("FIXME: FixedDay")
		}

		if constraint.FixedTime != nil {
			color.Red.Println("FIXME: FixedTime")
		}

		for _, day := range constraint.ExcludeDays {
			if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
				color.Red.Printf("Exam #%d planned on excluded day %s\n", constraint.Ancode, day.Format("02.01.06"))
			}
		}

		if len(constraint.PossibleDays) > 0 {
			possibleDaysOk := false
			var dayPlanned *time.Time
			for _, day := range constraint.PossibleDays {
				if day.Equal(time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local)) {
					possibleDaysOk = true
					dayPlanned = day
					break
				}
			}
			if !possibleDaysOk {
				color.Red.Printf("Exam #%d planned on day %s which is not a possible day\n", constraint.Ancode, dayPlanned.Format("02.01.06"))
			}
		}
	}

	return nil
}

func (p *Plexams) ValidateRoomsPerSlot() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating rooms per slot   --- ")

	slots := p.semesterConfig.Slots

	for _, slot := range slots {
		plannedRooms, err := p.dbClient.RoomsPlannedInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).
				Int("day", slot.DayNumber).
				Int("time", slot.SlotNumber).
				Msg("error while getting rooms planned in slot")
			return err
		}

		// color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println("    ---   check if planned rooms are allowed in slot  ")

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
			if plannedRoom.RoomName == "ONLINE" || plannedRoom.RoomName == "No Room" {
				break
			}

			isAllowed := false
			for _, allowedRoom := range allAllowedRooms {
				if allowedRoom.Name == plannedRoom.RoomName {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				color.Red.Printf("Room %s is not allowed in slot (%d,%d)\n", plannedRoom.RoomName, slot.DayNumber, slot.SlotNumber)
			}
		}

		// color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println("    ---   check if seatsPlanned <= seats  ")

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
				seats[plannedRoom.RoomName] = roomSeats{seatsPlanned: plannedRoom.SeatsPlanned, seats: p.GetRoomInfo(plannedRoom.RoomName).Seats}
			} else {
				seats[plannedRoom.RoomName] = roomSeats{seatsPlanned: plannedRoom.SeatsPlanned + entry.seatsPlanned, seats: p.GetRoomInfo(plannedRoom.RoomName).Seats}
			}
		}

		for roomName, roomSeats := range seats {
			if roomSeats.seatsPlanned > roomSeats.seats {
				color.Red.Printf("Room %s is overbooked in slot (%d, %d): %d seats planned, but only %d available \n",
					roomName, slot.DayNumber, slot.SlotNumber, roomSeats.seatsPlanned, roomSeats.seats)
			}
		}

	}

	return nil
}

func (p *Plexams) ValidateRoomsPerExam() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating rooms per exam   --- ")

	exams, err := p.ExamsInPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams in plan")
	}

	for _, exam := range exams {
		// check if each student has a room
		allStudentRegs := make([]*model.StudentReg, 0)
		for _, regs := range exam.Exam.StudentRegs {
			allStudentRegs = append(allStudentRegs, regs.StudentRegs...)
		}

		rooms, err := p.dbClient.RoomsForAncode(ctx, exam.Exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("andoce", exam.Exam.Ancode).Msg("cannot get rooms for ancode")
			return err
		}

		allStudentsInRooms := make([]*model.StudentReg, 0)
		for _, room := range rooms {
			if room.RoomName != "No Room" {
				allStudentsInRooms = append(allStudentsInRooms, room.Students...)
			}
		}

		for _, studentReg := range allStudentRegs {
			studentHasSeat := false
			for _, studentInRoom := range allStudentsInRooms {
				if studentReg.Mtknr == studentInRoom.Mtknr {
					studentHasSeat = true
					break
				}
			}
			if !studentHasSeat {
				color.Red.Printf("Student %s (%s) has no seat for exam %d. %s: %s in slot (%d,%d)\n",
					studentReg.Name, studentReg.Mtknr, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
					exam.Slot.DayNumber, exam.Slot.SlotNumber)
			}
		}

		// check if room constraints of exams are met
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil {
			if exam.Constraints.RoomConstraints.ExahmRooms {
				for _, room := range rooms {
					if !p.GetRoomInfo(room.RoomName).Exahm {
						color.Red.Printf("Is not Exahm-Room %s for exam %d. %s: %s in slot (%d,%d)\n",
							room.RoomName, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Lab {
				for _, room := range rooms {
					if !p.GetRoomInfo(room.RoomName).Lab {
						color.Red.Printf("Is not Lab %s for exam %d. %s: %s in slot (%d,%d)\n",
							room.RoomName, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.PlacesWithSocket {
				for _, room := range rooms {
					if !p.GetRoomInfo(room.RoomName).PlacesWithSocket && !p.GetRoomInfo(room.RoomName).Lab {
						color.Red.Printf("Is not Room with sockets %s for exam %d. %s: %s in slot (%d,%d)\n",
							room.RoomName, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}
				}
			}
		}

		// check rooms for NTAs
		// - needsRoomAlone okay
		// - TODO: enough time between usage

		if exam.Nta != nil {
			plannedRooms, err := p.dbClient.RoomsPlannedInSlot(ctx, exam.Slot.DayNumber, exam.Slot.SlotNumber)
			if err != nil {
				log.Error().Err(err).
					Int("day", exam.Slot.DayNumber).
					Int("time", exam.Slot.SlotNumber).
					Msg("error while getting rooms planned in slot")
				return err
			}

			for _, nta := range exam.Nta {
				if nta.Nta.NeedsRoomAlone {
					var roomForNta *model.RoomForExam
					for _, room := range plannedRooms {
						for _, student := range room.Students {
							if student.Mtknr == nta.Nta.Mtknr {
								roomForNta = room
								break
							}
						}
					}
				OUTER:
					for _, room := range plannedRooms {
						if room.RoomName == roomForNta.RoomName {
							for _, student := range room.Students {
								if student.Mtknr != nta.Nta.Mtknr {
									color.Red.Printf("NTA %s has room %s not alone for exam %d. %s: %s in slot (%d,%d)\n",
										nta.Nta.Name, room.RoomName, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
										exam.Slot.DayNumber, exam.Slot.SlotNumber)
									break OUTER
								}
							}
						}
					}
				} else /* do not need room alone */ {
					var roomForNta *model.RoomForExam
					for _, room := range plannedRooms {
						for _, student := range room.Students {
							if student.Mtknr == nta.Nta.Mtknr {
								roomForNta = room
								break
							}
						}
					}
					if roomForNta == nil {
						color.Red.Printf("NTA %s has no room for exam %d. %s: %s in slot (%d,%d)\n",
							nta.Nta.Name, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					} else {
						ntaDuration := int(math.Ceil(float64((exam.Exam.ZpaExam.Duration * (100 + nta.Nta.DeltaDurationPercent))) / 100))
						if roomForNta.Duration != ntaDuration {
							color.Red.Printf("NTA %s has room %s without correct duration %d for exam %d. %s: %s in slot (%d,%d): found %d\n",
								nta.Nta.Name, roomForNta.RoomName, ntaDuration, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
								exam.Slot.DayNumber, exam.Slot.SlotNumber, roomForNta.Duration)
						}
					}
				}
			}
		}

	}

	return nil
}

func (p *Plexams) ValidateRoomsTimeDistance() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating time distance of planned rooms   --- ")

	for _, day := range p.semesterConfig.Days {
		log.Debug().Interface("day", day).Msg("checking day")
		for i := range p.semesterConfig.Starttimes {
			if i == len(p.semesterConfig.Days)-1 {
				continue
			}
			slot1, slot2 := i+1, i+2
			log.Debug().Int("slot 1", slot1).Int("slot 2", slot2).Msg("checking slot")

			plannedRoomsSlot1, err := p.dbClient.RoomsPlannedInSlot(ctx, day.Number, slot1)
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

			plannedRoomsSlot2, err := p.dbClient.RoomsPlannedInSlot(ctx, day.Number, slot2)
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

					diff := 20 * time.Minute

					if startSlot2.Before(endSlot1.Add(diff)) {
						color.Red.Printf("Not enough time in room %s between slot (%d, %d) ends %s and slot (%d,%d) begins %s\n",
							roomName, day.Number, slot1, endSlot1.Format("15:04"), day.Number, slot2, startSlot2.Format("15:04"))
					}

				}
			}

		}

	}

	return nil
}
