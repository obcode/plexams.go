package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// RoomsWithFreeSeatsForSlot returns, for every room allowed in the slot, its
// capacity, how many seats are already used, the free seats and which exams use
// it — so the GUI can offer a (partly used) room for sharing, e.g. as a reserve.
func (p *Plexams) RoomsWithFreeSeatsForSlot(ctx context.Context, starttime time.Time) ([]*model.RoomWithFreeSeats, error) {
	slotRooms, err := p.RoomsForSlot(ctx, starttime)
	if err != nil {
		return nil, err
	}
	if slotRooms == nil {
		return []*model.RoomWithFreeSeats{}, nil
	}

	plannedRooms, err := p.PlannedRoomsInSlot(ctx, starttime)
	if err != nil {
		return nil, err
	}

	// students placed per room, and per (room, ancode)
	usedSeats := make(map[string]int)
	byRoomExam := make(map[string]map[int]int)
	for _, pr := range plannedRooms {
		usedSeats[pr.RoomName] += len(pr.StudentsInRoom)
		if byRoomExam[pr.RoomName] == nil {
			byRoomExam[pr.RoomName] = make(map[int]int)
		}
		byRoomExam[pr.RoomName][pr.Ancode] += len(pr.StudentsInRoom)
	}

	result := make([]*model.RoomWithFreeSeats, 0, len(slotRooms.RoomNames))
	for _, name := range slotRooms.RoomNames {
		room, err := p.RoomByName(ctx, name)
		if err != nil || room == nil {
			log.Error().Err(err).Str("room", name).Msg("cannot get room")
			continue
		}

		usedBy := make([]*model.RoomInSlotUsage, 0)
		for ancode, count := range byRoomExam[name] {
			module, examer := "", ""
			if exam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode); err == nil && exam != nil {
				module = exam.Module
				examer = exam.MainExamer
			}
			usedBy = append(usedBy, &model.RoomInSlotUsage{
				Ancode:       ancode,
				Module:       module,
				Examer:       examer,
				StudentCount: count,
			})
		}
		sort.SliceStable(usedBy, func(i, j int) bool { return usedBy[i].Ancode < usedBy[j].Ancode })

		result = append(result, &model.RoomWithFreeSeats{
			RoomName:  name,
			Seats:     room.Seats,
			UsedSeats: usedSeats[name],
			FreeSeats: room.Seats - usedSeats[name],
			Handicap:  room.Handicap,
			Exahm:     room.Exahm,
			Lab:       room.Lab,
			Seb:       room.Seb,
			UsedBy:    usedBy,
		})
	}

	sort.SliceStable(result, func(i, j int) bool { return result[i].RoomName < result[j].RoomName })
	return result, nil
}
