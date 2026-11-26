package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateInvigilatorRequirements(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilator-requirements", "Sperrtage und eigene Prüfungen der Aufsichten prüfen")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	v.step("Aufsichtssoll wird neu berechnet")
	invigilationTodos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		return nil, err
	}

	exams := p.newExamNamer(ctx, nil)
	for _, invigilator := range invigilationTodos.Invigilators {
		v.step("prüfe %s", invigilator.Teacher.Fullname)
		log.Debug().Str("name", invigilator.Teacher.Shortname).Msg("checking constraints")

		// days ok
		for _, invigilationDay := range invigilator.Todos.InvigilationDays {
			for _, excludedDay := range invigilator.Requirements.ExcludedDays {
				if invigilationDay == excludedDay {
					v.errorf(ref{InvigilatorID: ptr(invigilator.Teacher.ID)}.hint(hintExcludedDay),
						"%s ist am %s für eine Aufsicht eingeteilt, hat diesen Tag aber als Sperrtag.",
						invigilator.Teacher.Fullname, p.examDayLabelDE(invigilationDay))
				}
			}
		}

		// nur ein Raum oder Reserve
		invigilationSlots := set.NewSet[int64]() // keyed by the absolute start (Unix)
		for _, invigilation := range invigilator.Todos.Invigilations {
			if invigilation.Starttime == nil {
				continue
			}
			startKey := invigilation.Starttime.Unix()
			if invigilationSlots.Contains(startKey) {
				v.errorf(ref{InvigilatorID: ptr(invigilator.Teacher.ID), Starttime: invigilation.Starttime}.hint(hintReassign),
					"%s ist am %s für mehr als eine Aufsicht gleichzeitig eingeteilt.",
					invigilator.Teacher.Fullname, slotLabelDE(*invigilation.Starttime))
			}
			invigilationSlots.Add(startKey)

		}

		// wenn gleichzeitig Prüfung, dann nur self-invigilation
		ownExams, err := p.PlannedExamsByExamer(ctx, invigilator.Teacher.ID)
		if err != nil {
			log.Error().Err(err).Str("name", invigilator.Teacher.Shortname).Msg("cannot get exams")
		}

		for _, exam := range ownExams {
			for _, invigilation := range invigilator.Todos.Invigilations {
				if exam.PlanEntry != nil && exam.PlanEntry.Starttime != nil &&
					invigilation.Starttime != nil &&
					exam.PlanEntry.Starttime.Equal(*invigilation.Starttime) {
					when := slotLabelDE(*invigilation.Starttime)
					if invigilation.IsReserve {
						v.errorf(ref{Ancode: ptr(exam.Constraints.Ancode), InvigilatorID: ptr(invigilator.Teacher.ID), Starttime: invigilation.Starttime}.hint(hintReassign),
							"%s ist am %s als Reserveaufsicht eingeteilt, hat zur selben Zeit aber die eigene Prüfung %s.",
							invigilator.Teacher.Fullname, when, exams.name(exam.Constraints.Ancode))
					}

					roomsForExam, err := p.dbClient.PlannedRoomsForAncode(ctx, exam.Ancode)
					rooms := set.NewSet[string]()
					for _, room := range roomsForExam {
						rooms.Add(room.RoomName)
					}

					if err != nil {
						log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot get rooms for exam")
					} else {
						if rooms.Cardinality() > 1 {
							roomNames := rooms.ToSlice()
							sort.Strings(roomNames)
							v.errorf(ref{Ancode: ptr(exam.Constraints.Ancode), InvigilatorID: ptr(invigilator.Teacher.ID), Starttime: invigilation.Starttime}.hint(hintReassign),
								"%s ist am %s als Aufsicht eingeteilt, während die eigene Prüfung %s in mehreren Räumen läuft (%s). "+
									"Während einer eigenen Prüfung in mehreren Räumen übernimmt die prüfende Person keine Aufsicht.",
								invigilator.Teacher.Fullname, when, exams.name(exam.Constraints.Ancode), strings.Join(roomNames, ", "))
						}
					}

				}
			}
		}

	}

	return v.finish(), nil
}
func (p *Plexams) ValidateInvigilationDups(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilation-duplicates", "Doppelt gespeicherte Aufsichten suchen")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	v.step("Aufsichten werden gelesen")
	invigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all invigilations")
		return nil, err
	}

	type key struct {
		room  string
		start int64 // Unix seconds of the absolute start
	}

	invigilationsMap := make(map[key]*model.Invigilation)

	v.step("prüfe %d Aufsichten", len(invigilations))
	name := p.teacherNames(ctx)
	for _, invigilation := range invigilations {
		if invigilation.Starttime == nil {
			continue
		}
		var room string
		if invigilation.RoomName == nil {
			room = "null"
		} else {
			room = *invigilation.RoomName
		}
		key := key{
			room:  room,
			start: invigilation.Starttime.Unix(),
		}

		first, ok := invigilationsMap[key]
		if ok {
			v.errorf(ref{Room: invigilation.RoomName, InvigilatorID: ptr(invigilation.InvigilatorID), Starttime: invigilation.Starttime}.hint(hintDuplicate),
				"Für %s am %s sind zwei Aufsichten gespeichert: %s und %s.",
				invigilationPlaceDE(invigilation.RoomName), slotLabelDE(*invigilation.Starttime),
				name(first.InvigilatorID), name(invigilation.InvigilatorID))
		} else {
			invigilationsMap[key] = invigilation
		}
	}

	return v.finish(), nil
}

// TODO: NTA- und Reserve-Aufsicht (wenn NTA) nicht im folgenden Slot einteilen!
func (p *Plexams) ValidateInvigilatorSlots(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "invigilator-slots", "Prüfen, ob jeder Raum und jede Reserve eine Aufsicht hat")

	ctx := context.Background()
	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	// count rooms and reserves without and print number, keyed by calendar date
	// ("2006-01-02" sorts chronologically as a string).
	roomWithoutInvigilatorDay := make(map[string]int)
	slotWithoutReserveDay := make(map[string]int)
	maxInvigsMissingInOneSlot := make(map[string]int)
	dateByKey := make(map[string]time.Time)

	name := p.teacherNames(ctx)
	// all rooms and reserve max one invigilator
	for _, slot := range p.semesterConfig.Slots {
		invigsMissing := 0
		v.step("prüfe %s", slotLabelDE(slot.Starttime))

		rooms, err := p.plannedRoomNamesAt(ctx, slot.Starttime)
		if err != nil {
			log.Error().Err(err).Time("start", slot.Starttime).Msg("cannot get rooms for")
		}
		if len(rooms) == 0 {
			continue
		}
		dayKey := slot.Starttime.Format("2006-01-02")
		dateByKey[dayKey] = slot.Starttime

		invigilations, err := p.invigilationsAt(ctx, "reserve", slot.Starttime)
		if err != nil {
			log.Error().Err(err).Time("start", slot.Starttime).Msg("cannot get reserve invigilator")
		}

		if len(invigilations) == 0 {
			slotWithoutReserveDay[dayKey]++
			invigsMissing++
		} else if len(invigilations) > 1 {
			v.errorf(ref{Starttime: &slot.Starttime}.hint(hintDuplicate),
				"Am %s sind mehrere Reserveaufsichten eingeteilt: %s.",
				slotLabelDE(slot.Starttime), invigilatorNamesDE(invigilations, name))
		}

		for _, room := range rooms {
			invigilations, err := p.invigilationsAt(ctx, room, slot.Starttime)
			if err != nil {
				log.Error().Err(err).Time("start", slot.Starttime).Str("room", room).
					Msg("cannot get reserve invigilator")
			}
			if len(invigilations) == 0 {
				roomWithoutInvigilatorDay[dayKey]++
				invigsMissing++
			} else if len(invigilations) > 1 {
				v.warnf(ref{Room: ptr(room), Starttime: &slot.Starttime}.hint(hintDuplicate),
					"Für Raum %s am %s sind mehrere Aufsichten eingeteilt: %s.",
					room, slotLabelDE(slot.Starttime), invigilatorNamesDE(invigilations, name))
			}
		}
		if invigsMissing > maxInvigsMissingInOneSlot[dayKey] {
			maxInvigsMissingInOneSlot[dayKey] = invigsMissing
		}
	}

	if len(roomWithoutInvigilatorDay) > 0 || len(slotWithoutReserveDay) > 0 {
		keySet := set.NewSet[string]()
		for k := range roomWithoutInvigilatorDay {
			keySet.Add(k)
		}
		for k := range slotWithoutReserveDay {
			keySet.Add(k)
		}
		keys := keySet.ToSlice()

		sort.Strings(keys)

		for _, dayKey := range keys {
			roomsWithoutInvig := roomWithoutInvigilatorDay[dayKey]
			slotsWithoutReserve := slotWithoutReserveDay[dayKey]

			if roomsWithoutInvig+slotsWithoutReserve > 0 {
				dayStart := dateByKey[dayKey]
				v.warnf(ref{Starttime: &dayStart}.hint("Teile unter „Aufsichten einteilen“ ein, oder trage im „Aufsichtsplan“ die fehlenden Personen ein."),
					"Am %s sind %d Aufsichten noch offen: %d Räume ohne Aufsicht und %d Startzeiten ohne Reserveaufsicht (zur selben Zeit fehlen höchstens %d).",
					dayLabelDE(dayStart), roomsWithoutInvig+slotsWithoutReserve, roomsWithoutInvig, slotsWithoutReserve, maxInvigsMissingInOneSlot[dayKey])
			}
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateInvigilationsTimeDistance(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	timelag := p.generationTimelagMin(ctx)

	v := newValidation(reporter, "invigilations-time-distance",
		fmt.Sprintf("Pausen zwischen den Aufsichten prüfen (mindestens %d Minuten)", timelag))

	if ok, err := p.hasInvigilations(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoInvigilations), nil
	}

	v.step("Aufsichten werden gelesen")
	name := p.teacherNames(ctx)

	allInvigilations, err := p.dbClient.GetAllInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all invigilations")
	}

	// Group the invigilations per invigilator and sort each group by absolute
	// start time. The old positional "consecutive slot pair per day" scan becomes
	// a time-interval rule: for two invigilations of the same person on the same
	// calendar day the later one must not begin before the earlier one ends plus
	// the required time lag.
	byInvigilator := make(map[int][]*model.Invigilation)
	for _, invigilation := range allInvigilations {
		if invigilation.Starttime == nil {
			continue
		}
		byInvigilator[invigilation.InvigilatorID] = append(byInvigilator[invigilation.InvigilatorID], invigilation)
	}

	invigilatorIDs := make([]int, 0, len(byInvigilator))
	for id := range byInvigilator {
		invigilatorIDs = append(invigilatorIDs, id)
	}
	sort.Ints(invigilatorIDs)

	for _, id := range invigilatorIDs {
		invigs := byInvigilator[id]
		sort.Slice(invigs, func(i, j int) bool {
			return invigs[i].Starttime.Before(*invigs[j].Starttime)
		})

		for i := 0; i+1 < len(invigs); i++ {
			inv1, inv2 := invigs[i], invigs[i+1]
			// only compare invigilations on the same calendar day
			if !sameCalendarDay(*inv1.Starttime, *inv2.Starttime) {
				continue
			}
			v.step("prüfe %s am %s", name(id), dayLabelDE(*inv1.Starttime))

			realtime := inv1.Duration // TODO: calculate me

			// self- and reserve-invigilations occupy the whole slot: extend the
			// occupied time to the longest room duration at that start time.
			if inv1.IsSelfInvigilation || inv1.IsReserve {
				roomsAtStart, err := p.plannedRoomsAt(ctx, *inv1.Starttime)
				if err != nil {
					log.Error().Err(err).Time("start", *inv1.Starttime).Msg("cannot get rooms in slot")
				}
				for _, room := range roomsAtStart {
					if inv1.IsSelfInvigilation && (inv1.RoomName == nil || *inv1.RoomName != room.RoomName) {
						continue
					}
					if room.Duration > realtime {
						realtime = room.Duration
					}
				}
			}

			end1 := inv1.Starttime.Add(time.Duration(realtime) * time.Minute)

			if inv2.Starttime.Before(end1.Add(time.Duration(timelag) * time.Minute)) {
				gap := fmt.Sprintf("dazwischen liegen nur %.0f Minuten", inv2.Starttime.Sub(end1).Minutes())
				if inv2.Starttime.Before(end1) {
					gap = "die beiden überschneiden sich"
				}
				comment := ""
				if inv1.IsReserve {
					comment = " Die erste ist eine Reserveaufsicht; sie zählt bis zum Ende der längsten Prüfung zu dieser Zeit."
				}

				v.errorf(ref{InvigilatorID: ptr(id), Starttime: inv1.Starttime}.hint("Gib eine der beiden Aufsichten im „Aufsichtsplan“ an eine andere Person."),
					"%s hat am %s zu wenig Pause zwischen zwei Aufsichten: %s endet um %s, %s beginnt um %s; %s (nötig sind %d).%s",
					name(id), dayLabelDE(*inv1.Starttime), invigilationDutyDE(inv1.RoomName), end1.Format("15:04"),
					invigilationDutyDE(inv2.RoomName), inv2.Starttime.Format("15:04"), gap, timelag, comment)
			}
		}
	}

	return v.finish(), nil
}

// Hints shared by the invigilation findings.
const (
	hintReassign    = "Gib die Aufsicht im „Aufsichtsplan“ an eine andere Person."
	hintExcludedDay = "Gib die Aufsicht im „Aufsichtsplan“ an eine andere Person, oder prüfe die Sperrtage: " +
		"die aus dem ZPA stehen unter „Aufsichten mit Anforderungen“, zusätzliche unter „Aufsichts-Vorgaben“."
	hintDuplicate = "Prüfe im „Aufsichtsplan“, wer dort eingeteilt sein soll, und teile unter „Aufsichten einteilen“ neu ein. " +
		"Bleibt der Fehler, melde ihn der Plexams-Betreuung."
)

// teacherNames returns a lookup of the teachers' full names for messages; an
// unknown id is named as such instead of failing.
func (p *Plexams) teacherNames(ctx context.Context) func(id int) string {
	names := make(map[int]string)
	teachers, err := p.dbClient.GetTeachers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get teachers for invigilator names")
	}
	for _, t := range teachers {
		if t != nil && t.Fullname != "" {
			names[t.ID] = t.Fullname
		}
	}
	return func(id int) string {
		if n, ok := names[id]; ok {
			return n
		}
		return fmt.Sprintf("Unbekannte Aufsichtsperson (ZPA-Nr. %d)", id)
	}
}

// invigilatorNamesDE lists the names of the people assigned to invigilations.
func invigilatorNamesDE(invigilations []*model.Invigilation, name func(int) string) string {
	out := make([]string, 0, len(invigilations))
	for _, inv := range invigilations {
		out = append(out, name(inv.InvigilatorID))
	}
	return strings.Join(out, ", ")
}

// invigilationPlaceDE names the place of an invigilation: "Raum R1.006", or
// "die Reserveaufsicht" when there is no room.
func invigilationPlaceDE(room *string) string {
	if room == nil {
		return "die Reserveaufsicht"
	}
	return "Raum " + *room
}

// invigilationDutyDE names an invigilation: "die Aufsicht in Raum R1.006" or
// "die Reserveaufsicht".
func invigilationDutyDE(room *string) string {
	if room == nil {
		return "die Reserveaufsicht"
	}
	return "die Aufsicht in Raum " + *room
}

// dayLabelDE renders a date as e.g. "Mo 25.01.".
func dayLabelDE(t time.Time) string {
	return weekdaysDE[int(t.Weekday())] + " " + t.Format("02.01.")
}

// examDayLabelDE renders a 1-based exam-day number as its date, e.g. "Mo 25.01.".
func (p *Plexams) examDayLabelDE(day int) string {
	if day >= 1 && day <= len(p.semesterConfig.Days) {
		return dayLabelDE(p.semesterConfig.Days[day-1].Date)
	}
	return fmt.Sprintf("Prüfungstag %d", day)
}
