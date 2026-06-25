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

	roomInfos, err := p.roomInfoMapFromDB(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get rooms")
		return nil, err
	}

	// students that could not be assigned a real room during generation are kept
	// out of planned_rooms and recorded separately — report them as errors here.
	unplaced, err := p.dbClient.UnplacedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get unplaced exams")
		return nil, err
	}
	for _, u := range unplaced {
		what := "student(s)"
		if u.NtaMtknr != nil {
			what = "NTA student(s)"
		}
		v.errorf(ref{Ancode: ptr(u.Ancode), Day: ptr(u.Day), Slot: ptr(u.Slot)},
			"exam %d: %d %s without a room in slot (%d/%d)", u.Ancode, len(u.Mtknrs), what, u.Day, u.Slot)
	}

	// allowed rooms per slot, computed once (no stored cache anymore)
	roomsForSlots, err := p.roomsForSlotsMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot compute rooms for slots")
		return nil, err
	}

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

		// allowed rooms for this slot (empty for forbidden/pre-period slots — any
		// room planned there is then flagged below).
		allAllowedRooms, err := p.RoomsFromRoomNames(ctx, roomsForSlots[SlotNumber{day: slot.DayNumber, slot: slot.SlotNumber}])
		if err != nil {
			log.Error().Err(err).
				Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while getting rooms from names")
		}

		for _, plannedRoom := range plannedRooms {
			if plannedRoom.RoomName == "ONLINE" {
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
			// TODO: Remove this hack
			if strings.HasPrefix(plannedRoom.RoomName, "ONLINE") {
				continue
			}

			roomInfo := roomInfos[plannedRoom.RoomName]
			if roomInfo == nil {
				v.warnf(ref{Room: ptr(plannedRoom.RoomName), Day: ptr(slot.DayNumber), Slot: ptr(slot.SlotNumber)},
					"No room info found for planned room %s in slot (%d/%d); cannot check seats",
					plannedRoom.RoomName, slot.DayNumber, slot.SlotNumber)
				continue
			}

			entry := seats[plannedRoom.RoomName]
			seats[plannedRoom.RoomName] = roomSeats{
				seatsPlanned: len(plannedRoom.StudentsInRoom) + entry.seatsPlanned,
				seats:        roomInfo.Seats,
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
		log.Error().Err(err).Msg("cannot get entries from anny_bookings")
		return nil, err
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

	roomInfos, err := p.roomInfoMapFromDB(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get rooms")
		return nil, err
	}

PLANNEDROOM:
	for _, plannedRoom := range plannedRooms {

		roomInfo := roomInfos[plannedRoom.RoomName]
		if roomInfo == nil {
			v.warnf(ref{Room: ptr(plannedRoom.RoomName), Day: ptr(plannedRoom.Day), Slot: ptr(plannedRoom.Slot)},
				"No room info found for planned room %s in slot (%d/%d); cannot check whether it needs a request",
				plannedRoom.RoomName, plannedRoom.Day, plannedRoom.Slot)
			continue
		}
		if !roomInfo.NeedsRequest {
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
			v.errorf(ref{Room: ptr(b.Room), Day: ptr(b.Day), Slot: ptr(b.Slot)},
				"room %s is blocked in slot (%d/%d) but still planned there; regenerate rooms for exams",
				b.Room, b.Day, b.Slot)
		}
	}

	return v.finish(), nil
}

// ValidateRoomsEnoughSeats warns when an exam is packed too tightly, i.e. its
// normal rooms (NTA-alone rooms excluded, reserve rooms counted as free) leave
// fewer free seats than the buffer max(roomFreeSeatsMin, roomFreeSeatsPercent%).
func (p *Plexams) ValidateRoomsEnoughSeats(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-enough-seats", "validating enough free seats per exam")

	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	seats := make(map[string]int, len(rooms))
	handicap := make(map[string]bool, len(rooms))
	for _, room := range rooms {
		seats[room.Name] = room.Seats
		handicap[room.Name] = room.Handicap
	}

	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return nil, err
	}

	bufferByAncode := make(map[int]int)
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		capacity, normal, reserveSeats := 0, 0, 0
		for _, r := range exam.PlannedRooms {
			if r.NtaMtknr != nil {
				continue
			}
			if r.Reserve {
				reserveSeats += seats[r.RoomName]
				continue
			}
			normal += len(r.StudentsInRoom)
			capacity += seats[r.RoomName]
		}
		if normal == 0 {
			continue
		}
		v.step("checking free seats for exam %d", exam.Ancode)
		buffer := roomFreeSeatsBuffer(normal)
		bufferByAncode[exam.Ancode] = buffer
		free := capacity - normal + reserveSeats
		if free < buffer {
			module := ""
			if exam.ZpaExam != nil {
				module = exam.ZpaExam.Module
			}
			v.warnf(ref{Ancode: ptr(exam.Ancode)},
				"exam %d (%s): only %d free seat(s) for %d students; recommended buffer %d — too tight",
				exam.Ancode, module, free, normal, buffer)
		}
	}

	// shared rooms: a room used by several exams in a slot must have enough free
	// seats for the combined reserve buffers of those exams (the per-exam check
	// above counts each room's full capacity and would miss this).
	type slotRoom struct {
		day, slot int
		room      string
	}
	occupants := make(map[slotRoom]int)
	sharers := make(map[slotRoom]map[int]bool) // -> set of ancodes using it for normal/reserve
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		for _, r := range exam.PlannedRooms {
			key := slotRoom{r.Day, r.Slot, r.RoomName}
			occupants[key] += len(r.StudentsInRoom)
			if r.NtaMtknr == nil { // normal block or (empty) reserve
				if sharers[key] == nil {
					sharers[key] = make(map[int]bool)
				}
				sharers[key][exam.Ancode] = true
			}
		}
	}

	for key, ancodes := range sharers {
		if handicap[key.room] || len(ancodes) < 2 {
			continue // only shared, non-NTA rooms are interesting here
		}
		required := 0
		for ancode := range ancodes {
			required += bufferByAncode[ancode]
		}
		free := seats[key.room] - occupants[key]
		if free < required {
			v.warnf(ref{Room: ptr(key.room), Day: ptr(key.day), Slot: ptr(key.slot)},
				"room %s in slot (%d/%d) is shared by %d exams: only %d free seat(s) for a combined reserve of %d — too tight",
				key.room, key.day, key.slot, len(ancodes), free, required)
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

	roomInfos, err := p.roomInfoMapFromDB(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get rooms")
		return nil, err
	}

	waiverReasons, err := p.ntaRoomAloneWaiverReasons(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get nta room-alone waivers")
		return nil, err
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
			allStudentsInRooms = append(allStudentsInRooms, room.StudentsInRoom...)
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
			checkRoomConstraint := func(name, label string, ok func(*model.Room) bool) {
				for _, room := range exam.PlannedRooms {
					roomInfo := roomInfos[room.RoomName]
					if roomInfo == nil {
						v.warnf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"No room info found for room %s for exam %d. %s (%s) in slot (%d,%d); cannot check %s",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber, name)
						continue
					}
					if !ok(roomInfo) {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)},
							"%s %s for exam %d. %s (%s) in slot (%d,%d)",
							label, room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Exahm {
				checkRoomConstraint("exahm", "Is not an exahm room", func(r *model.Room) bool { return r.Exahm })
			}
			if exam.Constraints.RoomConstraints.Seb {
				checkRoomConstraint("seb", "Is not a seb room", func(r *model.Room) bool { return r.Seb })
			}
			if exam.Constraints.RoomConstraints.Lab {
				checkRoomConstraint("lab", "Is not a lab", func(r *model.Room) bool { return r.Lab })
			}
			if exam.Constraints.RoomConstraints.PlacesWithSocket {
				checkRoomConstraint("places with sockets", "Is not a room with places with sockets",
					func(r *model.Room) bool { return r.PlacesWithSocket || r.Lab })
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
									r := ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), StudentMtknr: ptr(nta.Mtknr), Day: ptr(exam.PlanEntry.DayNumber), Slot: ptr(exam.PlanEntry.SlotNumber)}
									if reason, ok := waiverReasons[ntaExamKey{nta.Mtknr, exam.Ancode}]; ok {
										v.infof(r,
											"NTA %s waives the room of their own for exam %d. %s (%s) in slot (%d,%d): %s",
											nta.Name, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
											exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber, reason)
									} else {
										v.errorf(r,
											"NTA %s has room %s not alone for exam %d. %s (%s) in slot (%d,%d)",
											nta.Name, room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
											exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
									}
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
