package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// RoomRequests returns all building-management room requests of the semester.
func (p *Plexams) RoomRequests(ctx context.Context) ([]*model.RoomRequest, error) {
	return p.dbClient.RoomRequests(ctx)
}

// SetRoomRequestApproved sets the approved flag of a room request (key:
// room/starttime). Errors if no such request exists.
func (p *Plexams) SetRoomRequestApproved(ctx context.Context, room string, starttime time.Time, approved bool) (*model.RoomRequest, error) {
	request, err := p.dbClient.SetRoomRequestApproved(ctx, room, starttime, approved)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("no room request for %s at %s", room, starttime.Format("02.01. 15:04"))
	}
	return request, nil
}

// SetRoomRequestActive activates/deactivates a room request (key: room/starttime).
// An inactive request is not used for room planning. Errors if it does not exist.
func (p *Plexams) SetRoomRequestActive(ctx context.Context, room string, starttime time.Time, active bool) (*model.RoomRequest, error) {
	request, err := p.dbClient.SetRoomRequestActive(ctx, room, starttime, active)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("no room request for %s at %s", room, starttime.Format("02.01. 15:04"))
	}
	return request, nil
}

// ApplyRoomRequestsPreview generates room requests from the current plan and
// replaces all existing ones (one-shot, no merge). Generated requests start
// active and not approved. To avoid clobbering requests that have already been
// approved (e.g. this semester's migrated data), it refuses to overwrite
// existing requests unless force is true. Returns the number written.
func (p *Plexams) ApplyRoomRequestsPreview(ctx context.Context, force bool) (int, error) {
	if err := p.generationAllowed(ctx, model.PlanningGateRooms); err != nil {
		return 0, err
	}
	existing, err := p.dbClient.RoomRequests(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 && !force {
		approved := 0
		for _, r := range existing {
			if r.Approved {
				approved++
			}
		}
		return 0, fmt.Errorf("%d room requests already exist (%d approved); "+
			"use force to discard them and regenerate", len(existing), approved)
	}

	preview, err := p.GenerateRoomRequestsPreview(ctx)
	if err != nil {
		return 0, err
	}

	requests := make([]*model.RoomRequest, 0, len(preview))
	for _, item := range preview {
		requests = append(requests, &model.RoomRequest{
			Room:      item.Room,
			Starttime: item.Starttime,
			From:      item.From,
			Until:     item.Until,
			Approved:  false,
			Active:    true,
		})
	}

	if err := p.dbClient.ReplaceAllRoomRequests(ctx, requests); err != nil {
		return 0, err
	}
	return len(requests), nil
}

// AddRoomRequest manually adds a single room request (key: room/starttime). It
// starts active and not approved. Errors if one already exists.
func (p *Plexams) AddRoomRequest(ctx context.Context, room string, starttime time.Time, from, until time.Time) (*model.RoomRequest, error) {
	existing, err := p.dbClient.GetRoomRequest(ctx, room, starttime)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("room request for %s at %s already exists", room, starttime.Format("02.01. 15:04"))
	}
	request := &model.RoomRequest{
		Room:      room,
		Starttime: &starttime,
		From:      from,
		Until:     until,
		Approved:  false,
		Active:    true,
	}
	if err := p.dbClient.AddRoomRequest(ctx, request); err != nil {
		return nil, err
	}
	return request, nil
}

// UpdateRoomRequestTime changes the time range of an existing room request, e.g.
// to extend it for an NTA (key: room/starttime). Errors if it does not exist.
func (p *Plexams) UpdateRoomRequestTime(ctx context.Context, room string, starttime time.Time, from, until time.Time) (*model.RoomRequest, error) {
	request, err := p.dbClient.UpdateRoomRequestTime(ctx, room, starttime, from, until)
	if err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("no room request for %s at %s", room, starttime.Format("02.01. 15:04"))
	}
	return request, nil
}
