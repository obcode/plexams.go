package plexams

import (
	"context"
	"encoding/json"
	"os"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

type ExportNtas struct {
	Name     string `json:"name"`
	Duration int    `json:"duration"`
}

type ExportPlannedRooms struct {
	MainExamer       string       `json:"mainExamer"`
	MainExamerID     int          `json:"mainExamerID"`
	Module           string       `json:"module"`
	Room             string       `json:"room"`
	Date             string       `json:"date"`
	Starttime        string       `json:"starttime"`
	NumberOfStudents int          `json:"numberOfStudents"`
	Duration         int          `json:"duration"`
	MaxDuration      int          `json:"maxDuration"`
	Invigilator      string       `json:"invigilator"`
	Ntas             []ExportNtas `json:"ntas"`
}

func (p *Plexams) ExportPlannedRooms(jsonfile string) error {
	ctx := context.Background()
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
	}

	exportPlannedRooms := make([]*ExportPlannedRooms, 0)
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)

		plannedRoomsMap := make(map[string][]*model.PlannedRoom)
		for _, plannedRoom := range exam.PlannedRooms {
			if _, ok := plannedRoomsMap[plannedRoom.RoomName]; !ok {
				plannedRoomsMap[plannedRoom.RoomName] = make([]*model.PlannedRoom, 0)
			}
			plannedRoomsMap[plannedRoom.RoomName] = append(plannedRoomsMap[plannedRoom.RoomName], plannedRoom)
		}

		for roomName, plannedRooms := range plannedRoomsMap {
			invigilator, err := p.GetInvigilatorInSlot(ctx, roomName, exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.Ancode).Str("room", roomName).
					Msg("cannot get invigilator for room")
				return err
			}

			numberOfStudents := 0
			maxDuration := 0
			ntas := make([]ExportNtas, 0)

			for _, plannedRoom := range plannedRooms {
				numberOfStudents += len(plannedRoom.StudentsInRoom)
				if plannedRoom.Duration > maxDuration {
					maxDuration = plannedRoom.Duration
				}
				if plannedRoom.NtaMtknr != nil && *plannedRoom.NtaMtknr != "" {
					nta, err := p.NtaByMtknr(ctx, *plannedRoom.NtaMtknr)
					if err != nil {
						log.Error().Err(err).Str("mtknr", *plannedRoom.NtaMtknr).Msg("cannot get nta")
						return err
					}
					ntas = append(ntas, ExportNtas{
						Name:     nta.Name,
						Duration: plannedRoom.Duration,
					})
				}
			}

			exportPlannedRooms = append(exportPlannedRooms, &ExportPlannedRooms{
				MainExamer:       exam.ZpaExam.MainExamer,
				MainExamerID:     exam.ZpaExam.MainExamerID,
				Module:           exam.ZpaExam.Module,
				Room:             roomName,
				Date:             starttime.Local().Format("02.01.2006"),
				Starttime:        starttime.Local().Format("15:04"),
				NumberOfStudents: numberOfStudents,
				Duration:         exam.ZpaExam.Duration,
				MaxDuration:      maxDuration,
				Invigilator:      invigilator.Shortname,
				Ntas:             ntas,
			})
		}
	}

	file, err := os.Create(jsonfile)
	if err != nil {
		log.Error().Err(err).Msg("cannot create JSON file")
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(exportPlannedRooms); err != nil {
		log.Error().Err(err).Msg("cannot encode JSON")
		return err
	}

	return nil
}
