package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ValidateZPADateTimes(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "zpa-date-times", "vergleiche Termine mit dem ZPA")

	v.step("fetching exams from ZPA")
	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	exams := p.zpa.client.GetExams()
	examsMap := make(map[int]*model.ZPAExam)

	for _, exam := range exams {
		examsMap[exam.AnCode] = exam
	}

	v.step("fetching planned exams from db")
	plannedExams, err := p.PlannedExams(context.Background())
	if err != nil {
		return nil, err
	}

	for _, plannedExam := range plannedExams {
		if plannedExam.Ancode >= 1000 {
			continue
		}
		v.step("checking exam %d. %s (%s)",
			plannedExam.Ancode, plannedExam.ZpaExam.Module, plannedExam.ZpaExam.MainExamer)
		zpaExam := examsMap[plannedExam.ZpaExam.AnCode]
		delete(examsMap, plannedExam.ZpaExam.AnCode)

		shouldHaveNoTimeAndDate := false
		if plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			shouldHaveNoTimeAndDate = true
		}

		if zpaExam == nil {
			log.Error().Int("ancode", plannedExam.ZpaExam.AnCode).Str("examer", plannedExam.ZpaExam.MainExamer).
				Str("module", plannedExam.ZpaExam.Module).Msg("zpa exam not found")
			continue
		}

		plannedExamDate := "-"
		plannedExamStarttime := "-"
		if !shouldHaveNoTimeAndDate && plannedExam.PlanEntry != nil && plannedExam.PlanEntry.Starttime != nil {
			starttime := *plannedExam.PlanEntry.Starttime
			plannedExamDate = starttime.Format("2006-01-02")
			plannedExamStarttime = starttime.Format("15:04:05")
		}

		var plannedStart *time.Time
		if plannedExam.PlanEntry != nil {
			plannedStart = plannedExam.PlanEntry.Starttime
		}
		if zpaExam.Date != plannedExamDate ||
			zpaExam.Starttime != plannedExamStarttime {
			v.errorf(ref{Ancode: ptr(zpaExam.AnCode), Starttime: plannedStart}.hint("Unter „ZPA-Import & Veröffentlichung“ den aktuellen Stand erneut ins ZPA übertragen und danach noch einmal prüfen."),
				"Im ZPA steht ein anderer Termin für %d. %s (%s): in Plexams geplant am %s um %s, im ZPA am %s um %s.",
				zpaExam.AnCode, zpaExam.Module, zpaExam.MainExamer,
				plannedExamDate, plannedExamStarttime, zpaExam.Date, zpaExam.Starttime)
		}
	}

	for _, zpaExam := range examsMap {
		if zpaExam.Date != "-" || zpaExam.Starttime != "-" {
			v.errorf(ref{Ancode: ptr(zpaExam.AnCode)}.hint("Prüfen, ob die Prüfung geplant werden soll. Wenn nicht, den Plan erneut ins ZPA übertragen, damit der Termin dort verschwindet."),
				"Im ZPA hat %d. %s (%s) den Termin %s %s, in Plexams ist die Prüfung aber nicht geplant.",
				zpaExam.AnCode, zpaExam.Module, zpaExam.MainExamer, zpaExam.Date, zpaExam.Starttime)
		}
	}

	return v.finish(), nil
}

func (p *Plexams) ValidateZPARooms(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "zpa-rooms", "vergleiche Räume mit dem ZPA")

	v.step("fetching exams from ZPA")
	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
	if err != nil {
		return nil, err
	}

	v.step("fetching planned exams from db")
	plannedExams, err := p.PlannedExams(context.Background())
	if err != nil {
		return nil, err
	}

	// check if plexams data is on zpa
	for _, plannedExam := range plannedExams {
		if plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			continue
		}
		v.step("checking exam %d. %s (%s)",
			plannedExam.Ancode, plannedExam.ZpaExam.Module, plannedExam.ZpaExam.MainExamer)

		roomsForAncode, err := p.dbClient.PlannedRoomsForAncode(context.Background(), plannedExam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", plannedExam.Ancode).Msg("cannot get planned rooms for ancode")
		}
		var plannedStart *time.Time
		if plannedExam.PlanEntry != nil {
			plannedStart = plannedExam.PlanEntry.Starttime
		}
		for _, room := range roomsForAncode {
			found := false
			for _, zpaExam := range plannedExamsFromZPA {
				if room.Ancode == zpaExam.Ancode &&
					roomNameOK(room.RoomName, zpaExam.RoomName) &&
					room.Duration == zpaExam.Duration &&
					room.Handicap == zpaExam.IsHandicap &&
					room.Reserve == zpaExam.IsReserve &&
					(len(room.StudentsInRoom) <= zpaExam.Number || // if more than one NTA in the room
						zpaExam.RoomName == "ONLINE") {
					found = true
					break
				}
			}
			if !found {
				v.errorf(ref{Ancode: ptr(plannedExam.Ancode), Room: ptr(room.RoomName), Starttime: plannedStart}.hint("Unter „ZPA-Import & Veröffentlichung“ den aktuellen Stand erneut ins ZPA übertragen und danach noch einmal prüfen."),
					"Raum %s für %d. %s (%s) steht in Plexams, fehlt aber im ZPA.",
					room.RoomName, plannedExam.Ancode, plannedExam.ZpaExam.Module, plannedExam.ZpaExam.MainExamer)
			}
		}

	}

	// TODO: check if zpa data is in plexams

	return v.finish(), nil
}

func (p *Plexams) ValidateZPAInvigilators(reporter Reporter) (*model.ValidationReport, error) {
	v := newValidation(reporter, "zpa-invigilators", "vergleiche Aufsichten mit dem ZPA")

	v.step("fetching exams from ZPA")
	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
	if err != nil {
		return nil, err
	}

	v.step("fetching planned exams from db")
	plannedExams, err := p.PlannedExams(context.Background())
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	// check if plexams data is on zpa
	for _, plannedExam := range plannedExams {
		if plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			continue
		}

		v.step("checking exam %d. %s (%s)",
			plannedExam.Ancode, plannedExam.ZpaExam.Module, plannedExam.ZpaExam.MainExamer)

		roomsForAncode := plannedExam.PlannedRooms
		// The invigilator lookup is keyed by the exam's absolute start time.
		if plannedExam.PlanEntry.Starttime == nil {
			continue
		}
		examStartTime := *plannedExam.PlanEntry.Starttime
		examStart := fmtStart(plannedExam.PlanEntry.Starttime)
		reserveInvigilator, err := p.GetInvigilatorAt(ctx, "reserve", examStartTime)
		if err != nil {
			log.Error().Err(err).Str("start", examStart).
				Msg("cannot get reserve invigilator for slot")
		}
		for _, room := range roomsForAncode {
			invigilator, err := p.GetInvigilatorAt(ctx, room.RoomName, examStartTime)
			if err != nil {
				log.Error().Err(err).Str("start", examStart).
					Msg("cannot get reserve invigilator for slot")
			}
			found := false
			for _, zpaExam := range plannedExamsFromZPA {
				if room.Ancode == zpaExam.Ancode &&
					roomNameOK(room.RoomName, zpaExam.RoomName) {
					if zpaExam.ReserveSupervisor != shorterName(reserveInvigilator.Shortname) {
						v.errorf(ref{Ancode: ptr(zpaExam.Ancode), Room: ptr(room.RoomName), Starttime: plannedExam.PlanEntry.Starttime}.hint("Unter „ZPA-Import & Veröffentlichung“ den aktuellen Stand erneut ins ZPA übertragen und danach noch einmal prüfen."),
							"%d. %s (%s), %s %s: Im ZPA steht als Reserveaufsicht %s, in Plexams ist %s eingeteilt.",
							zpaExam.Ancode, zpaExam.Module, zpaExam.MainExamer, zpaExam.Date, zpaExam.Starttime,
							zpaExam.ReserveSupervisor, shorterName(reserveInvigilator.Shortname))
					}
					if zpaExam.Supervisor != shorterName(invigilator.Shortname) {
						v.errorf(ref{Ancode: ptr(zpaExam.Ancode), Room: ptr(room.RoomName), Starttime: plannedExam.PlanEntry.Starttime}.hint("Unter „ZPA-Import & Veröffentlichung“ den aktuellen Stand erneut ins ZPA übertragen und danach noch einmal prüfen."),
							"%d. %s (%s), %s %s: Im ZPA steht als Aufsicht %s, in Plexams ist %s eingeteilt.",
							zpaExam.Ancode, zpaExam.Module, zpaExam.MainExamer, zpaExam.Date, zpaExam.Starttime,
							zpaExam.Supervisor, shorterName(invigilator.Shortname))
					}
					found = true
				}
			}
			if !found {
				v.errorf(ref{Ancode: ptr(plannedExam.Ancode), Room: ptr(room.RoomName), Starttime: plannedExam.PlanEntry.Starttime}.hint("Unter „ZPA-Import & Veröffentlichung“ den aktuellen Stand erneut ins ZPA übertragen und danach noch einmal prüfen."),
					"%d. %s (%s), %s: Im ZPA fehlt diese Prüfung, ihr Raum oder ihre Aufsicht.",
					plannedExam.Ancode, plannedExam.ZpaExam.Module, plannedExam.ZpaExam.MainExamer,
					examStart)
			}
		}

	}

	// TODO: check if zpa data is in plexams

	return v.finish(), nil
}

func roomNameOK(roomPlexams, roomZPA string) bool {
	return roomPlexams == roomZPA ||
		(strings.HasPrefix(roomPlexams, "ONLINE") && roomZPA == "ONLINE")
}

func shorterName(name string) string {
	parts := strings.Split(name, ",")
	if len(parts) != 2 {
		return name
	}

	lastname := strings.TrimSpace(parts[0])
	firstname := strings.TrimSpace(parts[1])

	if len(firstname) == 0 {
		return lastname
	}

	return fmt.Sprintf("%s, %s.", lastname, string(firstname[0]))
}
