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
	v := newValidation(reporter, "rooms-per-slot", "prüfe die Räume je Startzeit (verfügbar und genug Plätze)")

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
	names := p.newExamNamer(ctx, nil)
	for _, u := range unplaced {
		nta := ""
		if u.NtaMtknr != nil {
			nta = " Darunter ist eine Person mit NTA, die einen eigenen Raum braucht."
		}
		v.errorf(ref{Ancode: ptr(u.Ancode), Starttime: u.Starttime}.hint("Unter „Räume generieren“ noch einmal generieren. Reichen die Räume zu dieser Zeit nicht, weitere Räume beschaffen (Raum-Anfrage beim Gebäudemanagement oder Anny-Buchung) oder auf „Raumplanung“ von Hand einen Raum vorplanen."),
			"%s am %s: %d Studierende haben beim Generieren keinen Raum bekommen.%s",
			names.name(u.Ancode), startDE(u.Starttime), len(u.Mtknrs), nta)
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
		startStr := slotLabelDE(g.start)

		v.step("prüfe %s mit %d Räumen", startStr, len(g.rooms))

		// allowed rooms for this start (empty for forbidden/pre-period times — any room
		// planned there is then flagged below).
		allowedRooms := allowedByStart[key]

		for _, plannedRoom := range g.rooms {
			if plannedRoom.RoomName == "ONLINE" {
				continue
			}
			if allowedRooms == nil || !allowedRooms.Contains(plannedRoom.RoomName) {
				v.errorf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime}.hint("Unter „Räume generieren“ die Räume neu generieren. Ist der Raum dort vorgeplant (📌), auf „Raumplanung“ die Vorplanung lösen, oder für diese Zeit eine Raum-Anfrage bzw. Anny-Buchung anlegen."),
					"Raum %s ist am %s eingeplant, steht zu dieser Zeit aber nicht zur Verfügung (nicht angefragt oder gebucht, gesperrt oder deaktiviert).", plannedRoom.RoomName, startStr)
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
				v.warnf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime}.hint("Unter Semester → „Räume“ den Raum mit seiner Platzzahl anlegen, oder die Räume neu generieren."),
					"Raum %s ist am %s eingeplant, fehlt aber in den Raum-Stammdaten; die Platzzahl lässt sich nicht prüfen.",
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
				v.errorf(ref{Room: ptr(roomName), Starttime: &g.start}.hint("Unter „Räume generieren“ die Räume neu generieren. Liegt es an vorgeplanten Räumen (📌), auf „Raumplanung“ die Vorplanung für diesen Raum anpassen."),
					"Raum %s ist am %s überbelegt: %d Studierende eingeplant, aber nur %d Plätze vorhanden.",
					roomName, startStr, roomSeats.seatsPlanned, roomSeats.seats)
			}
		}

	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsNeedRequest(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-need-request", "prüfe, ob anzufragende Räume angefragt sind")

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
			v.warnf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime}.hint("Unter Semester → „Räume“ den Raum mit seiner Platzzahl anlegen, oder die Räume neu generieren."),
				"Raum %s ist am %s eingeplant, fehlt aber in den Raum-Stammdaten; ob er angefragt werden muss, lässt sich nicht prüfen.",
				plannedRoom.RoomName, startDE(plannedRoom.Starttime))
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
		startStr := slotLabelDE(startTime)
		endTime := startTime.Add(time.Duration(plannedRoom.Duration) * time.Minute)

		v.step("prüfe Raum %s am %s", plannedRoom.RoomName, startStr)

		for _, timerange := range roomTimetables[plannedRoom.RoomName] {
			if timerange.From.Before(startTime) && timerange.Until.After(endTime) {
				log.Debug().Str("room", plannedRoom.RoomName).Msg("found reservation")

				if !timerange.Approved {
					v.warnf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime}.hint("Sobald das Gebäudemanagement zusagt, auf „Raum-Anfragen (Gebäudemanagement)“ die Anfrage als genehmigt markieren. Bei einer Absage die Anfrage dort auf inaktiv setzen und die Räume neu generieren."),
						"Die Raum-Anfrage für Raum %s am %s ist noch nicht genehmigt.",
						plannedRoom.RoomName, startStr)
				}

				continue PLANNEDROOM
			}
		}

		v.errorf(ref{Room: ptr(plannedRoom.RoomName), Starttime: plannedRoom.Starttime}.hint("Auf „Raum-Anfragen (Gebäudemanagement)“ eine Anfrage für diese Zeit anlegen oder die bestehende verlängern. Bei T-Bau-Räumen die Buchung in Anny anlegen und unter „Anny-Buchungen“ neu abrufen. Soll der Raum nicht verwendet werden, die Räume neu generieren."),
			"Raum %s ist am %s eingeplant (bis %s) und muss angefragt werden, aber keine Raum-Anfrage oder Anny-Buchung deckt diese Zeit ab.",
			plannedRoom.RoomName, startStr, endTime.Format("15:04"))
	}

	return v.finish(), nil
}

// ValidateRoomsBlocked warns when a room that is blocked for a slot is still
// planned in that slot (the block only takes effect on the next rooms-for-exams
// run, so this surfaces the inconsistency until then).
func (p *Plexams) ValidateRoomsBlocked(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-blocked", "prüfe gesperrte Räume gegen die Raumplanung")

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
		startStr := slotLabelDE(*b.Starttime)
		v.step("prüfe die Sperre von Raum %s am %s", b.Room, startStr)
		if planned[b.Room+"@"+startKey(*b.Starttime)] {
			v.errorf(ref{Room: ptr(b.Room), Starttime: b.Starttime}.hint("Unter „Räume generieren“ die Räume neu generieren. Ist der Raum dort vorgeplant (📌), vorher auf „Raumplanung“ die Vorplanung lösen. Gilt die Sperre nicht mehr, sie auf „Raumplanung“ aufheben."),
				"Raum %s ist am %s gesperrt, ist dort aber noch eingeplant.",
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
	v := newValidation(reporter, "rooms-enough-seats", "prüfe die Platzreserve je Prüfung")

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
		v.step("prüfe die freien Plätze von Ancode %d", exam.Ancode)
		buffer := roomcalc.FreeSeatsBuffer(normal)
		bufferByAncode[exam.Ancode] = buffer
		free := capacity - normal + reserveSeats
		if free < buffer {
			var examStart *time.Time
			if exam.PlanEntry != nil {
				examStart = exam.PlanEntry.Starttime
			}
			v.warnf(ref{Ancode: ptr(exam.Ancode), Starttime: examStart}.hint("Unter „Räume generieren“ die Räume neu generieren. Oder auf „Raumplanung“ mit „➕ Raum vorplanen“ einen weiteren Raum für die Prüfung vorplanen."),
				"%s am %s ist knapp belegt: nur %d freie Plätze bei %d Studierenden, empfohlen sind mindestens %d als Reserve.",
				examName(exam), startDE(examStart), free, normal, buffer)
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
			v.warnf(ref{Room: ptr(key.room), Starttime: &start, RelatedAncodes: sortedAncodes(ancodes)}.hint("Unter „Räume generieren“ die Räume neu generieren. Oder auf „Raumplanung“ mit „➕ Raum vorplanen“ einen weiteren Raum für die Prüfung vorplanen."),
				"Raum %s wird am %s von %d Prüfungen geteilt und ist knapp belegt: nur %d freie Plätze, als Reserve sind zusammen %d empfohlen.",
				key.room, slotLabelDE(start), len(ancodes), free, required)
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateRoomsPerExam(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "rooms-per-exam", "prüfe die Räume je Prüfung")

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

		examStart := startDE(exam.PlanEntry.Starttime)
		name := examName(exam)

		v.step("prüfe die Räume von %s mit %d Studierenden, davon %d mit NTA",
			name, exam.StudentRegsCount, len(exam.Ntas))
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
				v.errorf(ref{Ancode: ptr(exam.Ancode), StudentMtknr: ptr(studentReg.Mtknr), Starttime: exam.PlanEntry.Starttime}.hint("Unter „Räume generieren“ die Räume neu generieren. Meist ist die Anmeldung erst nach dem Generieren dazugekommen, oder zu dieser Zeit war kein Platz mehr frei."),
					"%s (Matrikelnummer %s) hat bei %s am %s keinen Platz in einem Raum.",
					studentReg.Name, studentReg.Mtknr, name, examStart)
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
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Starttime: exam.PlanEntry.Starttime}.hint("Unter „Räume generieren“ die Räume neu generieren. Ist der Raum vorgeplant (📌), auf „Raumplanung“ einen der erlaubten Räume vorplanen, oder unter „Prüfungen auswählen & Vorgaben“ die erlaubten Räume erweitern."),
							"%s sitzt am %s in Raum %s, erlaubt sind laut Vorgabe aber nur: %s.",
							name, examStart, room.RoomName, strings.Join(exam.Constraints.RoomConstraints.AllowedRooms, ", "))
					}
				}
			}
			// needs is what the exam's Vorgabe asks of the room, e.g. "einen EXaHM-Raum".
			checkRoomConstraint := func(needs string, ok func(*model.Room) bool) {
				for _, room := range exam.PlannedRooms {
					roomInfo := roomInfos[room.RoomName]
					if roomInfo == nil {
						v.warnf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Starttime: exam.PlanEntry.Starttime}.hint("Unter Semester → „Räume“ den Raum mit seiner Platzzahl anlegen, oder die Räume neu generieren."),
							"%s sitzt am %s in Raum %s, der in den Raum-Stammdaten fehlt; ob er die Vorgabe (%s) erfüllt, lässt sich nicht prüfen.",
							name, examStart, room.RoomName, needs)
						continue
					}
					if !ok(roomInfo) {
						v.errorf(ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), Starttime: exam.PlanEntry.Starttime}.hint("Unter „Räume generieren“ die Räume neu generieren. Ist der Raum vorgeplant (📌), auf „Raumplanung“ einen passenden Raum vorplanen. Stimmt die Vorgabe nicht, sie unter „Prüfungen auswählen & Vorgaben“ ändern."),
							"%s braucht laut Vorgabe %s, sitzt am %s aber in Raum %s.",
							name, needs, examStart, room.RoomName)
					}
				}
			}
			if exam.Constraints.RoomConstraints.Exahm {
				checkRoomConstraint("einen EXaHM-Raum", func(r *model.Room) bool { return r.Exahm })
			}
			if exam.Constraints.RoomConstraints.Seb {
				checkRoomConstraint("einen SEB-Raum", func(r *model.Room) bool { return r.Seb })
			}
			if exam.Constraints.RoomConstraints.Lab {
				checkRoomConstraint("ein Labor", func(r *model.Room) bool { return r.Lab })
			}
			if exam.Constraints.RoomConstraints.PlacesWithSocket {
				checkRoomConstraint("Plätze mit Steckdose",
					func(r *model.Room) bool { return r.PlacesWithSocket || r.Lab })
			}
		}

		// check rooms for NTAs
		// - needsRoomAlone okay
		if len(exam.Ntas) > 0 {
			v.step("prüfe die Räume der Studierenden mit NTA")
			for _, nta := range exam.Ntas {
				if nta.NeedsRoomAlone {
					var roomForNta *model.PlannedRoom
					for _, room := range exam.PlannedRooms {
						if room.NtaMtknr != nil && *room.NtaMtknr == nta.Mtknr {
							roomForNta = room
							break
						}
					}
					// No room-alone was planned for this NTA (e.g. its exclusive room could not be
					// placed and the student is in the unplaced list, reported separately). Nothing
					// to check here — skip instead of dereferencing a nil room.
					if roomForNta == nil {
						continue
					}
					// Hint: the NTA sits in an OWN (R-building) EXaHM room used only as a fallback
					// (e.g. R1.011) because no booked T-building NTA room (T3.021) was available in
					// the slot. A whole R-building room is then tied up for one student — booking
					// T3.021 for the slot frees it.
					if ri := roomInfos[roomForNta.RoomName]; ri != nil && ri.Exahm && ri.RequestWith == model.RoomRequestTypeNone {
						r := ref{Ancode: ptr(exam.Ancode), Room: ptr(roomForNta.RoomName), StudentMtknr: ptr(nta.Mtknr), Starttime: exam.PlanEntry.Starttime}
						v.infof(r.hint("Wenn möglich T3.021 für diese Zeit in Anny buchen, unter „Anny-Buchungen“ neu abrufen und die Räume neu generieren."),
							"%s (NTA) sitzt bei %s am %s allein im EXaHM-Raum %s im R-Bau, weil zu dieser Zeit kein gebuchter T-Bau-Raum für NTA (T3.021) frei war. Damit ist ein ganzer Raum für eine Person belegt.",
							nta.Name, name, examStart, roomForNta.RoomName)
					}
				OUTER:
					for _, room := range exam.PlannedRooms {
						if room.RoomName == roomForNta.RoomName {
							for _, mtknr := range room.StudentsInRoom {
								if mtknr != nta.Mtknr {
									r := ref{Ancode: ptr(exam.Ancode), Room: ptr(room.RoomName), StudentMtknr: ptr(nta.Mtknr), Starttime: exam.PlanEntry.Starttime}
									if reason, ok := waiverReasons[ntaExamKey{nta.Mtknr, exam.Ancode}]; ok {
										r.Code = FindingNTARoomWaived
										v.infof(r,
											"%s (NTA) verzichtet bei %s am %s auf den eigenen Raum. Begründung: %s",
											nta.Name, name, examStart, reason)
									} else {
										r.Code = FindingNTARoomNotAlone
										v.errorf(r.hint("Unter „Räume generieren“ die Räume neu generieren. Verzichtet die Person auf den eigenen Raum, das hier mit „Verzicht akzeptieren“ und einer Begründung festhalten."),
											"%s (NTA) hat Anspruch auf einen eigenen Raum, sitzt bei %s am %s in Raum %s aber nicht allein.",
											nta.Name, name, examStart, room.RoomName)
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
		fmt.Sprintf("prüfe die Zeit zwischen zwei Prüfungen im selben Raum (mindestens %d Minuten)", timelag))

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

	v.step("prüfe %d Räume auf genug Zeit zum Umbauen", len(roomNames))
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
				v.errorf(ref{Room: ptr(name), Starttime: &cur.start}.hint("Unter „Räume generieren“ die Räume neu generieren. Liegt es an einem vorgeplanten Raum (📌), auf „Raumplanung“ für eine der beiden Prüfungen einen anderen Raum vorplanen."),
					"Zu wenig Zeit in Raum %s: die vorige Prüfung endet am %s, die nächste beginnt um %s. Dazwischen liegen %g Minuten, nötig sind %g.",
					name, slotLabelDE(prev.end), cur.start.Format("15:04"),
					cur.start.Sub(prev.end).Minutes(), required.Minutes())
			}
		}
	}

	return v.finish(), nil
}

// startDE renders an optional exam start for a finding, e.g. "Mo 25.01. 10:30".
func startDE(t *time.Time) string {
	if t == nil {
		return "(ohne Termin)"
	}
	return slotLabelDE(*t)
}

// examName names an assembled exam as "Modul (Prüfer:in, Ancode 123)".
func examName(exam *model.PlannedExam) string {
	if exam.ZpaExam == nil {
		return fmt.Sprintf("Ancode %d", exam.Ancode)
	}
	if exam.ZpaExam.MainExamer == "" {
		return fmt.Sprintf("%s (Ancode %d)", exam.ZpaExam.Module, exam.Ancode)
	}
	return fmt.Sprintf("%s (%s, Ancode %d)", exam.ZpaExam.Module, exam.ZpaExam.MainExamer, exam.Ancode)
}

// sortedAncodes returns the keys of an ancode set in ascending order.
func sortedAncodes(ancodes map[int]bool) []int {
	out := make([]int, 0, len(ancodes))
	for a := range ancodes {
		out = append(out, a)
	}
	sort.Ints(out)
	return out
}
