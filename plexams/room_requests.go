package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

// RoomRequests returns all building-management room requests of the semester.
func (p *Plexams) RoomRequests(ctx context.Context) ([]*model.RoomRequest, error) {
	return p.dbClient.RoomRequests(ctx)
}

// MigrateRoomRequestsFromConfig is the one-time import of
// roomConstraints.<room>.reservations from the semester config into the DB. All
// imported requests start active. Returns the number imported.
func (p *Plexams) MigrateRoomRequestsFromConfig(ctx context.Context) (int, error) {
	perRoom, err := p.reservationsFromConfig()
	if err != nil {
		return 0, err
	}

	requests := make([]*model.RoomRequest, 0)
	for room, timeRanges := range perRoom {
		for _, tr := range timeRanges {
			requests = append(requests, &model.RoomRequest{
				Room:     room,
				Day:      tr.DayNumber,
				Slot:     tr.SlotNumber,
				From:     tr.From,
				Until:    tr.Until,
				Approved: tr.Approved,
				Active:   true,
			})
		}
	}

	if err := p.dbClient.ReplaceAllRoomRequests(ctx, requests); err != nil {
		return 0, err
	}
	return len(requests), nil
}

// SetRoomRequestApproved sets the approved flag of a room request (key:
// room/day/slot). Errors if no such request exists.
func (p *Plexams) SetRoomRequestApproved(ctx context.Context, room string, day, slot int, approved bool) (*model.RoomRequest, error) {
	request, err := p.dbClient.SetRoomRequestApproved(ctx, room, day, slot, approved)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("no room request for %s in slot (%d,%d)", room, day, slot)
	}
	return request, nil
}

// SetRoomRequestActive activates/deactivates a room request (key: room/day/slot).
// An inactive request is not used for room planning. Errors if it does not exist.
func (p *Plexams) SetRoomRequestActive(ctx context.Context, room string, day, slot int, active bool) (*model.RoomRequest, error) {
	request, err := p.dbClient.SetRoomRequestActive(ctx, room, day, slot, active)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("no room request for %s in slot (%d,%d)", room, day, slot)
	}
	return request, nil
}
