package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidatePrePlannedExahmRooms(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "preplanned-exahm-rooms", "validating pre-planned exahm rooms (booked and enough seats)")

	exams := make([]*model.GeneratedExam, 0)
	generatedExams, err := p.dbClient.GetGeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).
			Msg("cannot get generated exams")
		return nil, err
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

	// allowed rooms per slot, computed once (no stored cache anymore)
	roomsForSlots, err := p.roomsForSlotsMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot compute rooms for slots")
		return nil, err
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
				v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.Name)},
					"Room %s for %d. %s (%s) is not SEB-Room",
					room.Name, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer)
			}

			if exam.Constraints.RoomConstraints.Exahm && !room.Exahm {
				v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.Name)},
					"Room %s for %d. %s (%s) is not EXaHM-Room",
					room.Name, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer)
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
			v.errorf(ref{Ancode: ptr(exam.Ancode)},
				"Exam %d. %s (%s) has no plan entry yet",
				exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer)
		} else {
			allowedRoomNames := roomsForSlots[SlotNumber{day: planEntry.DayNumber, slot: planEntry.SlotNumber}]
			for _, prePlannedRoom := range prePlannedRooms {
				found := false
				for _, roomInSlot := range allowedRoomNames {
					if prePlannedRoom.RoomName == roomInSlot {
						found = true
						break
					}
				}
				if !found {
					v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(prePlannedRoom.RoomName), Day: ptr(planEntry.DayNumber), Slot: ptr(planEntry.SlotNumber)},
						"Room %s for Exam %d. %s (%s) in slot (%d/%d) is not allowed",
						prePlannedRoom.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
						planEntry.DayNumber, planEntry.SlotNumber)
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
			v.errorf(ref{Ancode: ptr(exam.Ancode)},
				"Not enough seats for Exam %d. %s (%s): %d seats planned, but %d students",
				exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer, seats, exam.StudentRegsCount)
		}
	}

	return v.finish(), nil
}
