package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/roomcalc"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateRoomsPerSlot(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-per-slot", "validating rooms per slot (allowed and enough seats)")

	if ok, err := p.hasPlannedRooms(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoRooms), nil
	}

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
		v.errorf(ref{Ancode: ptr(u.Ancode), Starttime: u.Starttime},
			"exam %d: %d %s without a room at %s", u.Ancode, len(u.Mtknrs), what, fmtStart(u.Starttime))
	}

	// allowed room names per start time, computed once (no stored cache anymore).
	roomsForSlots, err := p.RoomsForSlots(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot compute rooms for slots")
		return nil, err
	}
	allowedByStart := make(map[string]set.Set[string], len(roomsForSlots))
	for _, rfs := range roomsForSlots {
		names := set.NewSet[string]()
		for _, n := range rfs.RoomNames {
			names.Add(n)
		}
		allowedByStart[startKey(rfs.Starttime)] = names
	}

	// group all planned rooms by their absolute start time.
	plannedRooms, err := p.dbClient.PlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return nil, err
	}
	type startGroup struct {
		start time.Time
		rooms []*model.PlannedRoom
	}
	groups := make(map[string]*startGroup)
	for _, pr := range plannedRooms {
		if pr.Starttime == nil {
			continue
		}
		key := startKey(*pr.Starttime)
		g := groups[key]
		if g == nil {
			g = &startGroup{start: *pr.Starttime}
			groups[key] = g
		}
		g.rooms = append(g.rooms, pr)
	}
	orderedGroups := make([]*startGroup, 0, len(groups))
	for _, g := range groups {
		orderedGroups = append(orderedGroups, g)
	}
	sort.Slice(orderedGroups, func(i, j int) bool { return orderedGroups[i].start.Before(orderedGroups[j].start) })

	for _, g := range orderedGroups {
		key := startKey(g.start)
		startStr := g.start.Format("02.01. 15:04")

		v.step("checking start %s with %d rooms", startStr, len(g.rooms))

		// allowed rooms for this start (empty for forbidden/pre-period times — any room
		// planned there is then flagged below).
		allowedRooms := allowedByStart[key]

		for _, plannedRoom := range g.rooms {
			if plannedRoom.RoomName == "ONLINE" {
				continue
			}
			if allowedRooms == nil || !allowedRooms.Contains(plannedRoom.RoomName) {
				v.errorf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime},
					"Room %s is not allowed at %s", plannedRoom.RoomName, startStr)
			}
		}

		type roomSeats struct {
			seatsPlanned, seats int
		}
		seats := make(map[string]roomSeats)

		for _, plannedRoom := range g.rooms {
			// TODO: Remove this hack
			if strings.HasPrefix(plannedRoom.RoomName, "ONLINE") {
				continue
			}

			roomInfo := roomInfos[plannedRoom.RoomName]
			if roomInfo == nil {
				v.warnf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime},
					"No room info found for planned room %s at %s; cannot check seats",
					plannedRoom.RoomName, startStr)
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
				v.errorf(ref{Room: ptr(roomName), Starttime: &g.start},
					"Room %s is overbooked at %s: %d seats planned, but only %d available",
					roomName, startStr, roomSeats.seatsPlanned, roomSeats.seats)
			}
		}

	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsNeedRequest(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-need-request", "validating rooms which need requests")

	if ok, err := p.hasPlannedRooms(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoRooms), nil
	}

	roomTimetables, err := p.GetReservations()
	if err != nil {
		log.Error().Err(err).Msg("cannot get reservations")
		return nil, err
	}

	annyRoomBookings, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get entries from anny_bookings")
		return nil, err
	}

	for _, annyRoomBooking := range annyRoomBookings {
		for _, roomName := range annyRoomBooking.Rooms {
			timeRanges, ok := roomTimetables[roomName]
			if !ok {
				timeRanges = make([]TimeRange, 0, 1)
			}
			roomTimetables[roomName] = append(timeRanges, TimeRange{
				From:     annyRoomBooking.From,
				Until:    annyRoomBooking.Until,
				Approved: annyRoomBooking.Approved,
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
			v.warnf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime},
				"No room info found for planned room %s at %s; cannot check whether it needs a request",
				plannedRoom.RoomName, fmtStart(plannedRoom.Starttime))
			continue
		}
		if !roomInfo.NeedsRequest {
			log.Debug().Str("room", plannedRoom.RoomName).Msg("room needs no request")
			continue
		}
		if plannedRoom.Starttime == nil {
			continue
		}

		startTime := *plannedRoom.Starttime
		startStr := startTime.Format("02.01. 15:04")
		endTime := startTime.Add(time.Duration(plannedRoom.Duration) * time.Minute)

		v.step("checking room %s at %s", plannedRoom.RoomName, startStr)

		for _, timerange := range roomTimetables[plannedRoom.RoomName] {
			if timerange.From.Before(startTime) && timerange.Until.After(endTime) {
				log.Debug().Str("room", plannedRoom.RoomName).Msg("found reservation")

				if !timerange.Approved {
					v.warnf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime},
						"Reservation for room %s at %s is not yet approved",
						plannedRoom.RoomName, startStr)
				}

				continue PLANNEDROOM
			}
		}

		v.errorf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime},
			"No Reservation for room %s found at %s",
			plannedRoom.RoomName, startStr)
	}

	return v.finish(), nil
}

// ValidateRoomsBlocked warns when a room that is blocked for a slot is still
// planned in that slot (the block only takes effect on the next rooms-for-exams
// run, so this surfaces the inconsistency until then).
func (p *Plexams) ValidateRoomsBlocked(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-blocked", "validating blocked rooms against planned rooms")

	if ok, err := p.hasPlannedRooms(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoRooms), nil
	}

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

	// key a planned room by its room name + absolute start time.
	planned := make(map[string]bool)
	for _, pr := range plannedRooms {
		if pr.Starttime == nil {
			continue
		}
		planned[pr.RoomName+"@"+startKey(*pr.Starttime)] = true
	}

	for _, b := range blocks {
		if b.Starttime == nil {
			continue
		}
		startStr := b.Starttime.Format("02.01. 15:04")
		v.step("checking block %s at %s", b.Room, startStr)
		if planned[b.Room+"@"+startKey(*b.Starttime)] {
			v.errorf(ref{Room: ptr(b.Room), Starttime: b.Starttime},
				"room %s is blocked at %s but still planned there; regenerate rooms for exams",
				b.Room, startStr)
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

	if ok, err := p.hasPlannedRooms(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoRooms), nil
	}

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
		buffer := roomcalc.FreeSeatsBuffer(normal)
		bufferByAncode[exam.Ancode] = buffer
		free := capacity - normal + reserveSeats
		if free < buffer {
			module := ""
			if exam.ZpaExam != nil {
				module = exam.ZpaExam.Module
			}
			var examStart *time.Time
			if exam.PlanEntry != nil {
				examStart = exam.PlanEntry.Starttime
			}
			v.warnf(ref{Ancode: ptr(exam.Ancode), Starttime: examStart},
				"exam %d (%s): only %d free seat(s) for %d students; recommended buffer %d — too tight",
				exam.Ancode, module, free, normal, buffer)
		}
	}

	// shared rooms: a room used by several exams at the same start time must have
	// enough free seats for the combined reserve buffers of those exams (the per-exam
	// check above counts each room's full capacity and would miss this).
	type startRoom struct {
		start string
		room  string
	}
	occupants := make(map[startRoom]int)
	sharers := make(map[startRoom]map[int]bool) // -> set of ancodes using it for normal/reserve
	startRep := make(map[startRoom]time.Time)   // representative start time for messages
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		for _, r := range exam.PlannedRooms {
			if r.Starttime == nil {
				continue
			}
			key := startRoom{startKey(*r.Starttime), r.RoomName}
			startRep[key] = *r.Starttime
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
			start := startRep[key]
			v.warnf(ref{Room: ptr(key.room), Starttime: &start},
				"room %s at %s is shared by %d exams: only %d free seat(s) for a combined reserve of %d — too tight",
				key.room, startRep[key].Format("02.01. 15:04"), len(ancodes), free, required)
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsPerExam(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-per-exam", "validating rooms per exam")

	if ok, err := p.hasPlannedRooms(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoRooms), nil
	}

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

		examStart := fmtStart(exam.PlanEntry.Starttime)

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
				v.errorf(ref{Ancode: ptr(exam.Ancode), StudentMtknr: ptr(studentReg.Mtknr), Starttime: exam.PlanEntry.Starttime},
					"Student %s (%s) has no seat for exam %d. %s (%s) at %s",
					studentReg.Name, studentReg.Mtknr, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
					examStart)
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
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Starttime: exam.PlanEntry.Starttime},
							"Room %s is not allowed for exam %d. %s (%s) at %s",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							examStart)
					}
				}
			}
			checkRoomConstraint := func(name, label string, ok func(*model.Room) bool) {
				for _, room := range exam.PlannedRooms {
					roomInfo := roomInfos[room.RoomName]
					if roomInfo == nil {
						v.warnf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Starttime: exam.PlanEntry.Starttime},
							"No room info found for room %s for exam %d. %s (%s) at %s; cannot check %s",
							room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							examStart, name)
						continue
					}
					if !ok(roomInfo) {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Starttime: exam.PlanEntry.Starttime},
							"%s %s for exam %d. %s (%s) at %s",
							label, room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
							examStart)
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
									r := ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), StudentMtknr: ptr(nta.Mtknr), Starttime: exam.PlanEntry.Starttime}
									if reason, ok := waiverReasons[ntaExamKey{nta.Mtknr, exam.Ancode}]; ok {
										v.infof(r,
											"NTA %s waives the room of their own for exam %d. %s (%s) at %s: %s",
											nta.Name, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
											examStart, reason)
									} else {
										v.errorf(r,
											"NTA %s has room %s not alone for exam %d. %s (%s) at %s",
											nta.Name, room.RoomName, exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
											examStart)
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
	timelag := p.generationTimelagMin(ctx)

	v := newValidation(reporter, "rooms-time-distance",
		fmt.Sprintf("validating time lag of planned rooms (%d minutes)", timelag))

	if ok, err := p.hasPlannedRooms(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoRooms), nil
	}

	plannedRooms, err := p.dbClient.PlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return nil, err
	}

	constraintsMap, err := p.ConstraintsMap(ctx)
	if err != nil {
		return nil, err
	}

	// Collect, per room, the end time of each distinct start it is used at (the longest
	// duration among the rows sharing that start). This is granularity-independent: it
	// works for any start times, not just consecutive grid slots. Alongside it we track
	// the largest EXTRA Vorlauf/Nachlauf any exam at that start demands beyond the ordinary
	// turnaround, so an exam that needs a bigger setup/teardown window widens the required
	// gap to its neighbours (see roomBuffers; extra = requested total − default 15 min).
	endByStart := make(map[string]map[time.Time]time.Time) // room -> start -> end
	preExtraByStart := make(map[string]map[time.Time]time.Duration)
	postExtraByStart := make(map[string]map[time.Time]time.Duration)
	for _, pr := range plannedRooms {
		if pr.Starttime == nil {
			continue
		}
		start := *pr.Starttime
		end := start.Add(time.Duration(pr.Duration) * time.Minute)
		if endByStart[pr.RoomName] == nil {
			endByStart[pr.RoomName] = make(map[time.Time]time.Time)
			preExtraByStart[pr.RoomName] = make(map[time.Time]time.Duration)
			postExtraByStart[pr.RoomName] = make(map[time.Time]time.Duration)
		}
		if cur, ok := endByStart[pr.RoomName][start]; !ok || end.After(cur) {
			endByStart[pr.RoomName][start] = end
		}
		pre, post := roomBuffers(constraintsMap[pr.Ancode])
		if e := pre - roomRequestBuffer; e > preExtraByStart[pr.RoomName][start] {
			preExtraByStart[pr.RoomName][start] = e
		}
		if e := post - roomRequestBuffer; e > postExtraByStart[pr.RoomName][start] {
			postExtraByStart[pr.RoomName][start] = e
		}
	}

	roomNames := make([]string, 0, len(endByStart))
	for name := range endByStart {
		roomNames = append(roomNames, name)
	}
	sort.Strings(roomNames)

	v.step("checking %d room(s) for enough turnaround time", len(roomNames))
	lag := time.Duration(timelag) * time.Minute
	for _, name := range roomNames {
		type use struct {
			start, end          time.Time
			preExtra, postExtra time.Duration
		}
		uses := make([]use, 0, len(endByStart[name]))
		for s, e := range endByStart[name] {
			uses = append(uses, use{s, e, preExtraByStart[name][s], postExtraByStart[name][s]})
		}
		sort.Slice(uses, func(i, j int) bool { return uses[i].start.Before(uses[j].start) })
		for i := 1; i < len(uses); i++ {
			prev, cur := uses[i-1], uses[i]
			// required gap = ordinary turnaround + the previous exam's extra teardown +
			// this exam's extra setup (both 0 for exams on the default 15-min buffer, so
			// default plans are unaffected).
			required := lag + prev.postExtra + cur.preExtra
			if cur.start.Before(prev.end.Add(required)) {
				v.errorf(ref{Room: ptr(name), Starttime: &cur.start},
					"Zu wenig Zeit in Raum %s: vorige Prüfung endet %s, nächste beginnt %s (%g Min dazwischen, benötigt %g)",
					name, prev.end.Format("02.01. 15:04"), cur.start.Format("02.01. 15:04"),
					cur.start.Sub(prev.end).Minutes(), required.Minutes())
			}
		}
	}

	return v.finish(), nil
}
