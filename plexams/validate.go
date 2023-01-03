package plexams

import (
	"context"
	"time"

	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateConflicts() error {
	ctx := context.Background()
	color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println(" ---   validating conflicts   --- ")

	planAncodeEntries, err := p.dbClient.PlanAncodeEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return err
	}

	studentRegs, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student registries per student")
		return err
	}

	for _, studentReg := range studentRegs {
		validateStudentReg(studentReg, planAncodeEntries)
	}
	return nil
}

func validateStudentReg(studentReg *model.StudentRegsPerStudent, planAncodeEntries []*model.PlanAncodeEntry) {
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
			// same slot
			if p[i].DayNumber == p[j].DayNumber &&
				p[i].SlotNumber == p[j].SlotNumber {
				color.Red.Printf("Same slot: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n",
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			} else
			// adjacent slots
			if p[i].DayNumber == p[j].DayNumber &&
				(p[i].SlotNumber+1 == p[j].SlotNumber ||
					p[i].SlotNumber-1 == p[j].SlotNumber) {
				color.Red.Printf("Adjacent slots: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n",
					p[i].Ancode, p[i].DayNumber, p[i].SlotNumber,
					p[j].Ancode, p[j].DayNumber, p[j].SlotNumber,
					studentReg.Student.Name, studentReg.Student.Program, studentReg.Student.Mtknr,
				)
			} else
			// same day
			if p[i].DayNumber == p[j].DayNumber {
				color.Yellow.Printf("Same day: ancodes %d (%d, %d) and %d (%d,%d) for student %s (%s/%s)\n",
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
			if plannedRoom.Room.Name == "ONLINE" || plannedRoom.Room.Name == "No Room" {
				break
			}

			isAllowed := false
			for _, allowedRoom := range allAllowedRooms {
				if allowedRoom.Name == plannedRoom.Room.Name {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				color.Red.Printf("Room %s is not allowed in slot (%d,%d)\n", plannedRoom.Room.Name, slot.DayNumber, slot.SlotNumber)
			}
		}

		// color.Style{color.FgRed, color.BgGreen, color.OpBold}.Println("    ---   check if seatsPlanned <= seats  ")

		type roomSeats struct {
			seatsPlanned, seats int
		}
		seats := make(map[string]roomSeats)

		for _, plannedRoom := range plannedRooms {
			entry, ok := seats[plannedRoom.Room.Name]
			if !ok {
				seats[plannedRoom.Room.Name] = roomSeats{seatsPlanned: plannedRoom.SeatsPlanned, seats: plannedRoom.Room.Seats}
			} else {
				seats[plannedRoom.Room.Name] = roomSeats{seatsPlanned: plannedRoom.SeatsPlanned + entry.seatsPlanned, seats: plannedRoom.Room.Seats}
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
			if room.Room.Name != "No Room" {
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

		// TODO: check if room constraints of exams are met
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil {
			if exam.Constraints.RoomConstraints.ExahmRooms {
				for _, room := range rooms {
					if !room.Room.Exahm {
						color.Red.Printf("Is not Exahm-Room %s for exam %d. %s: %s in slot (%d,%d)\n",
							room.Room.Name, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Lab {
				for _, room := range rooms {
					if !room.Room.Lab {
						color.Red.Printf("Is not Lab %s for exam %d. %s: %s in slot (%d,%d)\n",
							room.Room.Name, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.PlacesWithSocket {
				for _, room := range rooms {
					if !room.Room.PlacesWithSocket && !room.Room.Lab {
						color.Red.Printf("Is not Room with sockets %s for exam %d. %s: %s in slot (%d,%d)\n",
							room.Room.Name, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
							exam.Slot.DayNumber, exam.Slot.SlotNumber)
					}
				}
			}
		}

		// TODO: check rooms for NTAs
		// - enough time between usage
		// - needsRoomAlone okay

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
					for _, room := range plannedRooms {
						if room.Room.Name == roomForNta.Room.Name {
							for _, student := range room.Students {
								if student.Mtknr != nta.Nta.Mtknr {
									color.Red.Printf("NTA %s has room %s not alone for exam %d. %s: %s in slot (%d,%d)\n",
										nta.Nta.Name, room.Room.Name, exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module,
										exam.Slot.DayNumber, exam.Slot.SlotNumber)
								}
							}
						}
					}
				}
			}
		}

	}

	return nil
}
