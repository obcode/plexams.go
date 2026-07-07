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

	// The exam's absolute Starttime is the source of truth; deduplicate per start
	// time, keeping the longest exam in the room at that time.
	entriesMap := make(map[time.Time]*model.PlannedRoom)

	for _, plannedRoom := range plannedRooms {
		if plannedRoom.RoomName != roomName || plannedRoom.Starttime == nil {
			continue
		}
		start := *plannedRoom.Starttime
		if entry, okay := entriesMap[start]; !okay || plannedRoom.Duration > entry.Duration {
			entriesMap[start] = plannedRoom
		}
	}

	entriesForRoom := make([]*model.PlannedRoom, 0, len(entriesMap))
	for _, entry := range entriesMap {
		entriesForRoom = append(entriesForRoom, entry)
	}

	if len(entriesForRoom) == 0 {
		fmt.Printf("Raum %s ist nicht geplant\n", roomName)
		return nil
	}

	// Sort chronologically by absolute start time.
	sort.Slice(entriesForRoom, func(i, j int) bool {
		return entriesForRoom[i].Starttime.Before(*entriesForRoom[j].Starttime)
	})

	fmt.Printf("Planung für Raum %s:\n\n", roomName)

	for _, entry := range entriesForRoom {
		starttime := *entry.Starttime
		endtime := starttime.Add(time.Duration(entry.Duration) * time.Minute)
		fmt.Printf("- %s - %s (= %3d Minuten reine Prüfungszeit)\n",
			starttime.Format("02.01.06: 15:04"), endtime.Format("15:04 Uhr"), entry.Duration)
	}

	fmt.Println(`
Angegeben ist immer die reine Prüfungszeit,
d.h. der Raum sollte ca. 15 Minuten vorher verfügbar sein und
ist ca. 15 Minuten nach der Prüfung wieder verfügbar.`)

	return nil
}
