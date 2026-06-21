package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) ValidateRoomsPerSlot(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-per-slot", "validating rooms per slot (allowed and enough seats)")

	slots := p.semesterConfig.Slots

	for _, slot := range slots {

		plannedExams, err := p.dbClient.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).
				Int("day", slot.DayNumber).
				Int("time", slot.SlotNumber).
				Msg("error while getting exams planned in slot")
			return nil, err
		}

		plannedRooms := make([]*model.PlannedRoom, 0)
		for _, plannedExam := range plannedExams {
			plannedRooms = append(plannedRooms, plannedExam.PlannedRooms...)
		}

		v.step("checking slot (%d/%d) with %d rooms in %d exams",
			slot.DayNumber, slot.SlotNumber, len(plannedRooms), len(plannedExams))

		allowedRooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).
				Int("day", slot.DayNumber).
				Int("time", slot.SlotNumber).
				Msg("error while getting allowed rooms for slot")
			return nil, err
		}
		allAllowedRooms, err := p.RoomsFromRoomNames(ctx, allowedRooms.RoomNames)
		if err != nil {
			log.Error().Err(err).
				Interface("room names", allowedRooms.RoomNames).
				Msg("error while getting rooms from names")
		}

		for _, plannedRoom := range plannedRooms {
			if plannedRoom.RoomName == "ONLINE" {
				continue
			}

			if plannedRoom.RoomName == "No Room" {
				v.errorf(ref{Day: ptr(slot.DayNumber), Slot: ptr(slot.SlotNumber)},
					"No Room for %d students in slot (%d/%d)", len(plannedRoom.StudentsInRoom), slot.DayNumber, slot.SlotNumber)
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
				v.errorf(ref{Room: ptr(plannedRoom.RoomName), Day: ptr(slot.DayNumber), Slot: ptr(slot.SlotNumber)},
					"Room %s is not allowed in slot (%d/%d)", plannedRoom.RoomName, slot.DayNumber, slot.SlotNumber)
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
				v.errorf(ref{Room: ptr(roomName), Day: ptr(slot.DayNumber), Slot: ptr(slot.SlotNumber)},
					"Room %s is overbooked in slot (%d/%d): %d seats planned, but only %d available",
					roomName, slot.DayNumber, slot.SlotNumber, roomSeats.seatsPlanned, roomSeats.seats)
			}
		}

	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsNeedRequest(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-need-request", "validating rooms which need requests")

	roomTimetables, err := p.GetReservations()
	if err != nil {
		log.Error().Err(err).Msg("cannot get reservations")
		return nil, err
	}

	bookedEntries, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get entries from anny_bookings, fallback to booked entries in YAML")
		bookedEntries = nil
	}
	if len(bookedEntries) == 0 {
		bookedEntries, err = p.ExahmRoomsFromBooked()
		if err != nil {
			log.Error().Err(err).Msg("cannot get booked entries")
			return nil, err
		}
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

PLANNEDROOM:
	for _, plannedRoom := range plannedRooms {

		if !p.roomInfo[plannedRoom.RoomName].NeedsRequest {
			log.Debug().Str("room", plannedRoom.RoomName).Msg("room needs no request")
			continue
		}

		v.step("checking room %s in slot (%d/%d)", plannedRoom.RoomName, plannedRoom.Day, plannedRoom.Slot)

		startTime := p.getSlotTime(plannedRoom.Day, plannedRoom.Slot)
		endTime := startTime.Add(time.Duration(plannedRoom.Duration) * time.Minute)

		for _, timerange := range roomTimetables[plannedRoom.RoomName] {
			if timerange.From.Before(startTime) && timerange.Until.After(endTime) {
				log.Debug().Str("room", plannedRoom.RoomName).Msg("found reservation")

				if !timerange.Approved {
					v.warnf(ref{Room: ptr(plannedRoom.RoomName), Day: ptr(plannedRoom.Day), Slot: ptr(plannedRoom.Slot)},
						"Reservation for room %s found in slot (%d/%d) is not yet approved",
						plannedRoom.RoomName, plannedRoom.Day, plannedRoom.Slot)
				}

				continue PLANNEDROOM
			}
		}

		v.errorf(ref{Room: ptr(plannedRoom.RoomName), Day: ptr(plannedRoom.Day), Slot: ptr(plannedRoom.Slot)},
			"No Reservation for room %s found in slot (%d/%d)",
			plannedRoom.RoomName, plannedRoom.Day, plannedRoom.Slot)
	}

	return v.finish(), nil
}

// ValidateRoomsBlocked warns when a room that is blocked for a slot is still
// planned in that slot (the block only takes effect on the next rooms-for-exams
// run, so this surfaces the inconsistency until then).
func (p *Plexams) ValidateRoomsBlocked(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-blocked", "validating blocked rooms against planned rooms")

	blocks, err := p.dbClient.BlockedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get blocked rooms")
		return nil, err
	}
	if len(blocks) == 0 {
		return v.finish(), nil
	}

	plannedRooms, err := p.PlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return nil, err
	}

	type slotRoom struct {
		room string
		day  int
		slot int
	}
	planned := make(map[slotRoom]bool)
	for _, pr := range plannedRooms {
		planned[slotRoom{pr.RoomName, pr.Day, pr.Slot}] = true
	}

	for _, b := range blocks {
		v.step("checking block %s in slot (%d/%d)", b.Room, b.Day, b.Slot)
		if planned[slotRoom{b.Room, b.Day, b.Slot}] {
			v.warnf(ref{Room: ptr(b.Room), Day: ptr(b.Day), Slot: ptr(b.Slot)},
				"room %s is blocked in slot (%d/%d) but still planned there; regenerate rooms for exams",
				b.Room, b.Day, b.Slot)
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsPerExam(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-per-exam", "validating rooms per exam")

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

		v.step("checking rooms for %d. %s (%s) with %d students and %d ntas",
			exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer, exam.StudentRegsCount, len(exam.Ntas))
		// check if each student has a room
		allStudentRegs := make([]*model.EnhancedStudentReg, 0)
		for _, primussExam := range exam.PrimussExams {
			allStudentRegs = append(allStudentRegs, primussExam.StudentRegs...)
		}

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
				v.errorf(ref{Ancode: ptr(exam.Ancode), StudentMtknr: ptr(studentReg.Mtknr), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
					"Student %s (%s) has no seat for exam %d. %s (%s) in slot (%d,%d)",
					studentReg.Name, studentReg.Mtknr, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
					exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			}
		}

		// check if room constraints of exams are met
		if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil {
			if len(exam.Constraints.RoomConstraints.AllowedRooms) > 0 {
				allowedRooms := set.NewSet[string]()
				for _, room := range exam.Constraints.RoomConstraints.AllowedRooms {
					allowedRooms.Add(room)
				}
				for _, room := range exam.PlannedRooms {
					if !allowedRooms.Contains(room.RoomName) {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"Room %s is not allowed for exam %d. %s (%s) in slot (%d,%d)",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Exahm {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).Exahm {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"Is not an exahm room %s for exam %d. %s (%s) in slot (%d,%d)",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Seb {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).Seb {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"Is not a seb room %s for exam %d. %s (%s) in slot (%d,%d)",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Lab {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).Lab {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"Is not a lab %s for exam %d. %s (%s) in slot (%d,%d)",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.PlacesWithSocket {
				for _, room := range exam.PlannedRooms {
					if !p.GetRoomInfo(room.RoomName).PlacesWithSocket && !p.GetRoomInfo(room.RoomName).Lab {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"Is not a room with places with sockets %s for exam %d. %s (%s) in slot (%d,%d)",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
					}
				}
			}
		}

		// check rooms for NTAs
		// - needsRoomAlone okay
		if len(exam.Ntas) > 0 {
			v.step("checking rooms for ntas")
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
									v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), StudentMtknr: ptr(nta.Mtknr), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
										"NTA %s has room %s not alone for exam %d. %s (%s) in slot (%d,%d)",
										nta.Name, room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
										exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
									break OUTER
								}
							}
						}
					}
				}
			}
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsTimeDistance(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	timelag := viper.GetInt("rooms.timelag")

	v := newValidation(reporter, "rooms-time-distance",
		fmt.Sprintf("validating time lag of planned rooms (%d minutes)", timelag))

	for _, day := range p.semesterConfig.Days {
		v.step("checking day %d (%s)", day.Number, day.Date.Format("02.01.06"))

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
				return nil, err
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
				return nil, err
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
						return nil, err
					}
					endSlot1 := start.Add(time.Duration(maxDuration) * time.Minute)

					startSlot2, err := time.Parse("15:04", p.semesterConfig.Starttimes[i+1].Start)
					if err != nil {
						log.Error().Err(err).Str("starttime", p.semesterConfig.Starttimes[i].Start).Msg("cannot parse starttime")
						return nil, err
					}
					log.Debug().Str("room", roomName).Int("max duration", maxDuration).
						Str("starttime slot 1", p.semesterConfig.Starttimes[i].Start).
						Str("endtime slot 1", endSlot1.Format("15:04")).
						Str("starttime slot 2", startSlot2.Format("15:04")).
						Msg("checking")

					diff := time.Duration(timelag) * time.Minute

					if startSlot2.Before(endSlot1.Add(diff)) {
						v.errorf(ref{Room: ptr(roomName), Day: ptr(day.Number), Slot: ptr(slot2)},
							"Not enough time in room %s between slot (%d/%d) ends %s and slot (%d/%d) begins %s: %g minutes between",
							roomName, day.Number, slot1, endSlot1.Format("15:04"),
							day.Number, slot2, startSlot2.Format("15:04"),
							startSlot2.Sub(endSlot1).Minutes())
					}
				}
			}
		}
	}

	return v.finish(), nil
}
