package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// RequestRoomsInfo prints, read-only, which building-management rooms would be
// requested for which exams (the same dry-run the GUI shows via
// roomRequestsPreview). It changes nothing: generating, approving and emailing
// the actual requests happens in the GUI / via the GraphQL API.
func (p *Plexams) RequestRoomsInfo() error {
	ctx := context.Background()
	preview, err := p.GenerateRoomRequestsPreview(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot generate room requests preview")
		return err
	}

	if len(preview) == 0 {
		fmt.Println("Keine Raum-Anfragen über das Gebäudemanagement nötig.")
		return nil
	}

	fmt.Println("Probelauf Raum-Anfragen (Gebäudemanagement) — verändert nichts.")
	fmt.Println("Übernehmen und Mail verschicken im GUI.")
	fmt.Println()
	fmt.Println("| Raum     | Datum    | von   | bis   | Stud. | Plätze | Prüfung |")
	fmt.Println("|----------|----------|-------|-------|-------|--------|---------|")
	for _, req := range preview {
		module := ""
		ancode := 0
		if req.Exam != nil && req.Exam.ZpaExam != nil {
			module = req.Exam.ZpaExam.Module
			ancode = req.Exam.Ancode
		}
		fmt.Printf("| %-8s | %s | %s | %s | %5d | %6d | %d. %s |\n",
			req.Room,
			req.From.Format("02.01.06"),
			req.From.Format("15:04"),
			req.Until.Format("15:04"),
			req.Students,
			req.Seats,
			ancode, module,
		)
	}
	return nil
}

// PlannedRoomInfo prints the planned room for a given room name.
func (p *Plexams) PlannedRoomInfo(roomName string) error {
	ctx := context.Background()
	plannedRooms, err := p.PlannedRooms(ctx)

	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return err
	}

	type slot struct {
		day  int
		slot int
	}

	entriesMap := make(map[slot]*model.PlannedRoom)

	for _, plannedRoom := range plannedRooms {
		if plannedRoom.RoomName == roomName {
			entry, okay := entriesMap[slot{plannedRoom.Day, plannedRoom.Slot}]
			if !okay {
				entriesMap[slot{plannedRoom.Day, plannedRoom.Slot}] = plannedRoom
			} else {
				if plannedRoom.Duration > entry.Duration {
					// If the new entry has a longer duration, replace the existing one
					entriesMap[slot{plannedRoom.Day, plannedRoom.Slot}] = plannedRoom
				}
			}
		}
	}

	entriesForRoom := make([]*model.PlannedRoom, 0)
	for _, entry := range entriesMap {
		entriesForRoom = append(entriesForRoom, entry)
	}

	if len(entriesForRoom) == 0 {
		fmt.Printf("Raum %s ist nicht geplant\n", roomName)
		return nil
	}

	// Sort entriesForRoom by Day and Slot
	sort.Slice(entriesForRoom, func(i, j int) bool {
		if entriesForRoom[i].Day != entriesForRoom[j].Day {
			return entriesForRoom[i].Day < entriesForRoom[j].Day
		}
		return entriesForRoom[i].Slot < entriesForRoom[j].Slot
	})

	starttimes := make(map[int]map[int]time.Time)
	for _, day := range p.semesterConfig.Days {
		dayMap := make(map[int]time.Time)
		starttimes[day.Number] = dayMap
		for i, slot := range p.semesterConfig.Starttimes {
			starttime, err := time.Parse("15:04", slot.Start)
			if err != nil {
				log.Error().Err(err).Str("time-string", slot.Start).Msg("cannot parse time")
				return err
			}
			realStartTime := time.Date(
				day.Date.Year(), day.Date.Month(), day.Date.Day(),
				starttime.Hour(), starttime.Minute(), 0, 0, day.Date.Location())
			dayMap[i+1] = realStartTime
		}
	}

	fmt.Printf("Planung für Raum %s:\n\n", roomName)

	for _, entry := range entriesForRoom {
		starttime := starttimes[entry.Day][entry.Slot]
		endtime := starttime.Add(time.Duration(entry.Duration) * time.Minute) // 90 minutes for the exam slot
		fmt.Printf("- %s - %s (= %3d Minuten reine Prüfungszeit)\n",
			starttime.Format("02.01.06: 15:04"), endtime.Format("15:04 Uhr"), entry.Duration)
	}

	fmt.Println(`
Angegeben ist immer die reine Prüfungszeit,
d.h. der Raum sollte ca. 15 Minuten vorher verfügbar sein und
ist ca. 15 Minuten nach der Prüfung wieder verfügbar.`)

	return nil
}
