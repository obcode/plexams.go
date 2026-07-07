package plexams

import (
	"context"
	"fmt"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
)

// formatBlockedRoomTime renders a room block's absolute start time for user-facing
// messages (real date/time instead of an ordinal slot label).
func formatBlockedRoomTime(t *time.Time) string {
	if t == nil {
		return "unbekannter Zeitpunkt"
	}
	return t.Format("02.01. 15:04")
}

// BlockedRooms returns all room-slot blocks of the semester.
func (p *Plexams) BlockedRooms(ctx context.Context) ([]*model.BlockedRoom, error) {
	return p.dbClient.BlockedRooms(ctx)
}

// BlockRoomForSlot marks a room as not usable in one slot (e.g. otherwise
// occupied). It is honored by the rooms-for-slots / rooms-for-exams generation.
// The block always succeeds; a conflict with an already-planned room only
// surfaces as a warning on the next generation.
func (p *Plexams) BlockRoomForSlot(ctx context.Context, room string, starttime time.Time, reason *string) (*model.BlockedRoom, error) {
	dbRoom, err := p.dbClient.RoomByName(ctx, room)
	if err != nil {
		return nil, err
	}
	if dbRoom == nil {
		return nil, fmt.Errorf("room %s not found", room)
	}
	block := &model.BlockedRoom{
		Starttime: &starttime,
		Room:      room,
		Reason:    reason,
	}
	if err := p.dbClient.BlockRoomForSlot(ctx, block); err != nil {
		return nil, err
	}
	return block, nil
}

// UnblockRoomForSlot removes a room-slot block. Errors if no such block exists.
func (p *Plexams) UnblockRoomForSlot(ctx context.Context, room string, starttime time.Time) (bool, error) {
	removed, err := p.dbClient.UnblockRoomForSlot(ctx, room, starttime)
	if err != nil {
		return false, err
	}
	if !removed {
		return false, fmt.Errorf("no block for room %s at %s", room, formatBlockedRoomTime(&starttime))
	}
	return true, nil
}

// RemovePrePlannedRoom removes a pre-planned room from an exam (mtknr nil = the
// room for the normal students). Errors if no such pre-planned room exists.
func (p *Plexams) RemovePrePlannedRoom(ctx context.Context, ancode int, roomName string, mtknr *string) (bool, error) {
	removed, err := p.dbClient.RemovePrePlannedRoomFromExam(ctx, ancode, roomName, mtknr)
	if err != nil {
		return false, err
	}
	if !removed {
		return false, fmt.Errorf("no pre-planned room %s for ancode %d", roomName, ancode)
	}
	return true, nil
}

// applyRoomBlocks removes the blocked (room, slot) pairs from slotsWithRoomNames
// so the rooms-for-slots cache never offers a blocked room. If a blocked room is
// currently planned in that slot, it warns (the block still wins).
func (p *Plexams) applyRoomBlocks(ctx context.Context, slotsWithRoomNames map[time.Time]set.Set[string], reporter Reporter) error {
	blocks, err := p.dbClient.BlockedRooms(ctx)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return nil
	}

	plannedInSlot := make(map[time.Time]set.Set[string])
	plannedRooms, err := p.dbClient.PlannedRooms(ctx)
	if err != nil {
		return err
	}
	for _, pr := range plannedRooms {
		if pr.Starttime == nil {
			continue
		}
		key := *pr.Starttime
		if _, ok := plannedInSlot[key]; !ok {
			plannedInSlot[key] = set.NewSet[string]()
		}
		plannedInSlot[key].Add(pr.RoomName)
	}

	for _, b := range blocks {
		if b.Starttime == nil {
			continue
		}
		key := *b.Starttime
		if roomNames, ok := slotsWithRoomNames[key]; ok {
			roomNames.Remove(b.Room)
		}
		when := formatBlockedRoomTime(b.Starttime)
		if planned, ok := plannedInSlot[key]; ok && planned.Contains(b.Room) {
			reporter.Warnf(aurora.Sprintf(
				aurora.Red("room %s is blocked at %s but currently planned there; it will be dropped on the next rooms-for-exams run"),
				b.Room, when))
		} else {
			reporter.Println(aurora.Sprintf(aurora.Yellow("room %s blocked at %s"), b.Room, when))
		}
	}
	return nil
}
