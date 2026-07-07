package plexams

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/roomcalc"
	"github.com/obcode/plexams.go/plexams/rooms"
	"github.com/rs/zerolog/log"
)

// PrepareRoomForExams assigns rooms to all planned exams and stores the result in
// planned_rooms. The allowed rooms per slot are computed live once (see
// prepareRoomsCfg / computeRoomsForSlots) — there is no separate rooms-for-slots
// step or stored cache anymore.
func (p *Plexams) PrepareRoomForExams(ctx context.Context, reporter Reporter) error {
	if err := p.generationAllowed(ctx, model.PlanningGateRooms); err != nil {
		return err
	}
	cfg, err := p.prepareRoomsCfg(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get prepare rooms config")
		return err
	}

	reporter.Println(aurora.Sprintf(aurora.Cyan("preparing rooms for exams")))
	examRooms := make([]*model.PlannedRoom, 0)
	unplaced := make([]*model.UnplacedExam, 0)
	// The slots are ordered day-by-day, then by start time; the first slot seen for a
	// new calendar date is its earliest one (the former SlotNumber == 1), which resets
	// the blocked-room carryover.
	lastDate := ""
	for _, slot := range p.semesterConfig.Slots {
		date := slot.Starttime.Format("2006-01-02")
		cfg.Slot = slot
		cfg.IsNewDay = date != lastDate
		lastDate = date
		slotRooms, slotUnplaced, err := rooms.PrepareForSlot(ctx, p.dbClient, cfg, reporter)
		if err != nil {
			log.Error().Err(err).Time("starttime", slot.Starttime).
				Msg("error while preparing rooms for exams in slot")
			continue
		}
		// The absolute slot start is the persisted source of truth; day/slot are
		// derived from it on read. All rooms/unplaced of this call belong to this slot.
		st := slot.Starttime
		for _, r := range slotRooms {
			r.Starttime = &st
		}
		for _, u := range slotUnplaced {
			u.Starttime = &st
		}
		examRooms = append(examRooms, slotRooms...)
		unplaced = append(unplaced, slotUnplaced...)
	}

	if err := p.dbClient.ReplacePlannedRooms(ctx, examRooms); err != nil {
		return err
	}
	if err := p.dbClient.ReplaceUnplacedExams(ctx, unplaced); err != nil {
		return err
	}
	p.markCondition(ctx, condRoomsAssigned)
	if len(unplaced) > 0 {
		reporter.StopProgress(fmt.Sprintf("%d planned rooms written, %d exam(s) with unplaced students", len(examRooms), len(unplaced)))
	} else {
		reporter.StopProgress(fmt.Sprintf("%d planned rooms written", len(examRooms)))
	}
	return nil
}

// ResetRoomsForExams drops the generated room plan (planned_rooms) so that only
// the pre-planning (rooms_pre_planned) remains; a re-generation re-applies the
// pre-planned rooms. Blocked while the room plan is published.
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

// prepareRoomsCfg builds the static room-allocation configuration for a full
// generation run. The per-slot mutable state (Slot / exams / availableRooms / …)
// is filled in by rooms.PrepareForSlot; the outer loop sets cfg.Slot per slot.
func (p *Plexams) prepareRoomsCfg(ctx context.Context) (*rooms.Cfg, error) {
	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return nil, err
	}

	roomInfo := make(map[string]*model.Room)
	for _, room := range allRooms {
		roomInfo[room.Name] = room
	}

	blocks, err := p.dbClient.BlockedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get blocked rooms")
		return nil, err
	}
	// Blocked rooms and allowed rooms are keyed on the slot's absolute start time
	// (canonicalized via rooms.StartKey); a block whose time is off the grid simply
	// never matches a slot lookup.
	blockedRooms := make(map[rooms.SlotKey]set.Set[string])
	for _, b := range blocks {
		if b.Starttime == nil {
			continue
		}
		key := rooms.StartKey(*b.Starttime)
		if _, ok := blockedRooms[key]; !ok {
			blockedRooms[key] = set.NewSet[string]()
		}
		blockedRooms[key].Add(b.Room)
	}

	roomsForSlotsByStart, err := p.roomsForSlotsMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot compute rooms for slots")
		return nil, err
	}
	roomsForSlots := make(map[rooms.SlotKey][]string, len(roomsForSlotsByStart))
	for start, v := range roomsForSlotsByStart {
		roomsForSlots[rooms.StartKey(start)] = v
	}

	cfg := &rooms.Cfg{
		RoomInfo:              roomInfo,
		PrePlannedRooms:       p.prePlannedRooms(ctx, roomInfo),
		AdditionalSeats:       p.additionalSeats(ctx),
		BlockedRooms:          blockedRooms,
		ExactSeatRooms:        make(map[int]map[string]bool),
		RoomsForSlots:         roomsForSlots,
		SlotBlockMinutes:      int(slotBlockDuration(p.semesterConfig.Starttimes).Minutes()),
		RoomTurnaroundMinutes: int(roomRequestBuffer.Minutes()),
	}

	log.Info().Interface("prePlannedRooms", cfg.PrePlannedRooms).Msg("prepareRoomsCfg initialized")

	return cfg, nil
}

// additionalSeats returns the extra seats to reserve per ancode, read from the
// per-exam room constraints (RoomConstraints.additionalSeats).
func (p *Plexams) additionalSeats(ctx context.Context) map[int]int {
	additionalSeats := make(map[int]int) // ancode -> seats

	constraints, err := p.Constraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints for additional seats")
		return additionalSeats
	}
	for _, c := range constraints {
		if c.RoomConstraints != nil && c.RoomConstraints.AdditionalSeats != nil && *c.RoomConstraints.AdditionalSeats > 0 {
			additionalSeats[c.Ancode] = *c.RoomConstraints.AdditionalSeats
		}
	}

	log.Debug().Interface("additionalSeats", additionalSeats).Msg("found additional seats")

	return additionalSeats
}

func (p *Plexams) prePlannedRooms(ctx context.Context, roomInfo map[string]*model.Room) map[int][]*model.PrePlannedRoom {
	prePlannedRoomsMap := make(map[int][]*model.PrePlannedRoom)
	prePlannedRooms, err := p.dbClient.PrePlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre-planned rooms")
		return nil
	}

	for _, room := range prePlannedRooms {
		if _, ok := prePlannedRoomsMap[room.Ancode]; !ok {
			prePlannedRoomsMap[room.Ancode] = make([]*model.PrePlannedRoom, 0, 1)
		}

		prePlannedRoomsMap[room.Ancode] = append(prePlannedRoomsMap[room.Ancode], room)
	}

	for _, rooms := range prePlannedRoomsMap {
		roomcalc.SortPrePlannedRooms(rooms, roomInfo)
	}

	return prePlannedRoomsMap
}
