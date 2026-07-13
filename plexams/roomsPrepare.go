package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/roomcalc"
	"github.com/rs/zerolog/log"
)

// Room generation is done by the constraint solver (see roomplan_build.go:
// GenerateRoomPlan). The former greedy per-slot allocator (PrepareRoomForExams +
// package plexams/rooms) was removed; only the reset and the pre-planned-rooms input
// helper (shared with the solver build) remain here.

// ResetRoomsForExams drops the generated room plan (planned_rooms) so that only the
// pre-planning (rooms_pre_planned) remains; a re-generation re-applies the pre-planned
// rooms. Blocked while the room plan is published.
func (p *Plexams) ResetRoomsForExams(ctx context.Context) error {
	if err := p.generationAllowed(ctx, model.PlanningGateRooms); err != nil {
		return err
	}
	if err := p.dbClient.ResetPlannedRooms(ctx); err != nil {
		return err
	}
	if err := p.dbClient.ResetUnplacedExams(ctx); err != nil {
		return err
	}
	p.unmarkCondition(ctx, condRoomsAssigned)
	return nil
}

// prePlannedRooms returns the manually pre-planned rooms grouped by ancode and sorted into
// fill order (NTA rooms first, then by descending seats, reserve last). Shared by the room
// solver build (buildRoomPlanProblem).
func (p *Plexams) prePlannedRooms(ctx context.Context, roomInfo map[string]*model.Room) map[int][]*model.PrePlannedRoom {
	prePlannedRoomsMap := make(map[int][]*model.PrePlannedRoom)
	prePlannedRooms, err := p.dbClient.PrePlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre-planned rooms")
		return nil
	}

	for _, room := range prePlannedRooms {
		prePlannedRoomsMap[room.Ancode] = append(prePlannedRoomsMap[room.Ancode], room)
	}

	for _, rooms := range prePlannedRoomsMap {
		roomcalc.SortPrePlannedRooms(rooms, roomInfo)
	}

	return prePlannedRoomsMap
}
