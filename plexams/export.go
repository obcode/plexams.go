package plexams

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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

// PlannedRoomsForExport builds the planned-rooms export data (one entry per
// room of each exam I planned, with examer, time, students, durations,
// invigilator and NTAs) used to generate the cover pages externally.
func (p *Plexams) PlannedRoomsForExport(ctx context.Context) ([]*ExportPlannedRooms, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
	}

	boolVal := false
	teacher, err := p.GetTeachers(ctx, &boolVal)
	if err != nil {
		log.Error().Err(err).Msg("cannot get teachers")
		return nil, err
	}

	teachers := make(map[int]*model.Teacher)
	for _, t := range teacher {
		teachers[t.ID] = t
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
				return nil, err
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
						return nil, err
					}
					ntas = append(ntas, ExportNtas{
						Name:     nta.Name,
						Duration: plannedRoom.Duration,
					})
				}
			}

			exportPlannedRooms = append(exportPlannedRooms, &ExportPlannedRooms{
				MainExamer:       teachers[exam.ZpaExam.MainExamerID].Shortname,
				MainExamerID:     exam.ZpaExam.MainExamerID,
				Module:           exam.ZpaExam.Module,
				Room:             roomName,
				Date:             starttime.Format("02.01.2006"),
				Starttime:        starttime.Format("15:04"),
				NumberOfStudents: numberOfStudents,
				Duration:         exam.ZpaExam.Duration,
				MaxDuration:      maxDuration,
				Invigilator:      teachers[invigilator.ID].Shortname,
				Ntas:             ntas,
			})
		}
	}

	return exportPlannedRooms, nil
}

// PlannedRoomsJSON returns the planned-rooms export as indented JSON bytes.
func (p *Plexams) PlannedRoomsJSON(ctx context.Context) ([]byte, error) {
	data, err := p.PlannedRoomsForExport(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(data, "", "  ")
}

// ExportPlannedRooms writes the planned-rooms export to a JSON file (CLI).
func (p *Plexams) ExportPlannedRooms(jsonfile string) error {
	b, err := p.PlannedRoomsJSON(context.Background())
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonfile, b, 0644); err != nil {
		log.Error().Err(err).Msg("cannot write JSON file")
		return err
	}
	return nil
}

// HTTPDownloadPlannedRooms serves the planned-rooms export JSON as a file
// download, so the GUI can fetch it to generate the cover pages externally.
func (p *Plexams) HTTPDownloadPlannedRooms(w http.ResponseWriter, r *http.Request) {
	b, err := p.PlannedRoomsJSON(r.Context())
	if err != nil {
		http.Error(w, "cannot build planned-rooms export: "+err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("%s_planned-rooms.json", strings.ReplaceAll(p.semester, " ", "_"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Msg("cannot write planned-rooms download")
	}
}
