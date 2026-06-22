package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

// NtaRoomAloneWaivers returns all accepted NTA room-alone waivers.
func (p *Plexams) NtaRoomAloneWaivers(ctx context.Context) ([]*model.NtaRoomAloneWaiver, error) {
	return p.dbClient.NtaRoomAloneWaivers(ctx)
}

// AddNtaRoomAloneWaiver accepts that an NTA gives up the room-alone right for one
// exam (key: mtknr/ancode), stored with a reason. Errors on an empty reason or an
// unknown student.
func (p *Plexams) AddNtaRoomAloneWaiver(ctx context.Context, mtknr string, ancode int, reason string) (*model.NtaRoomAloneWaiver, error) {
	if reason == "" {
		return nil, fmt.Errorf("a reason is required")
	}
	student, err := p.StudentByMtknr(ctx, mtknr)
	if err != nil {
		return nil, err
	}
	if student == nil {
		return nil, fmt.Errorf("student with mtknr %s not found", mtknr)
	}
	waiver := &model.NtaRoomAloneWaiver{Mtknr: mtknr, Ancode: ancode, Reason: reason}
	if err := p.dbClient.AddNtaRoomAloneWaiver(ctx, waiver); err != nil {
		return nil, err
	}
	return waiver, nil
}

// RemoveNtaRoomAloneWaiver removes a waiver (key: mtknr/ancode). Errors if none
// exists.
func (p *Plexams) RemoveNtaRoomAloneWaiver(ctx context.Context, mtknr string, ancode int) (bool, error) {
	removed, err := p.dbClient.RemoveNtaRoomAloneWaiver(ctx, mtknr, ancode)
	if err != nil {
		return false, err
	}
	if !removed {
		return false, fmt.Errorf("no room-alone waiver for %s and exam %d", mtknr, ancode)
	}
	return true, nil
}

// ntaRoomAloneWaiverReasons returns the waivers as a map keyed by mtknr+ancode →
// reason, for quick lookup during validation and email building.
func (p *Plexams) ntaRoomAloneWaiverReasons(ctx context.Context) (map[ntaExamKey]string, error) {
	waivers, err := p.dbClient.NtaRoomAloneWaivers(ctx)
	if err != nil {
		return nil, err
	}
	reasons := make(map[ntaExamKey]string, len(waivers))
	for _, w := range waivers {
		reasons[ntaExamKey{w.Mtknr, w.Ancode}] = w.Reason
	}
	return reasons, nil
}

type ntaExamKey struct {
	mtknr  string
	ancode int
}
