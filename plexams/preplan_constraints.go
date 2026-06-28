package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// SetPreplanExamConstraints stores the constraints of a pre-exam. It reuses the
// normal ConstraintsInput; the `sameSlot` ints reference other PRE-EXAM ids (not
// ancodes) and are kept symmetric (the link is mirrored onto the referenced
// pre-exams), so the order in which they are later connected to ZPA ancodes does
// not matter.
func (p *Plexams) SetPreplanExamConstraints(ctx context.Context, id int, input *model.ConstraintsInput) (*model.PreplanExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	all, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	exists := make(map[int]bool, len(all))
	for _, pe := range all {
		exists[pe.ID] = true
	}

	constraints := preplanConstraintsFromInput(input)
	// keep only same-slot references to other existing pre-exams (no self-link)
	constraints.SameSlot = filterPreplanIDs(constraints.SameSlot, id, exists)

	oldSame := []int{}
	if preExam.Constraints != nil {
		oldSame = preExam.Constraints.SameSlot
	}

	preExam.Constraints = constraints
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preExam); err != nil {
		return nil, err
	}

	// mirror the same-slot link: add id to the now-referenced pre-exams, remove it
	// from the ones no longer referenced.
	newSet := intSet(constraints.SameSlot)
	oldSet := intSet(oldSame)
	for other := range newSet {
		if !oldSet[other] {
			if err := p.addPreplanSameSlot(ctx, other, id); err != nil {
				return nil, err
			}
		}
	}
	for other := range oldSet {
		if !newSet[other] {
			if err := p.removePreplanSameSlot(ctx, other, id); err != nil {
				return nil, err
			}
		}
	}

	return preExam, nil
}

func (p *Plexams) addPreplanSameSlot(ctx context.Context, id, add int) error {
	pe, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil || pe == nil {
		return err
	}
	if pe.Constraints == nil {
		pe.Constraints = &model.Constraints{}
	}
	if !intSet(pe.Constraints.SameSlot)[add] {
		pe.Constraints.SameSlot = append(pe.Constraints.SameSlot, add)
		sort.Ints(pe.Constraints.SameSlot)
		_, err = p.dbClient.ReplacePreplanExam(ctx, pe)
	}
	return err
}

func (p *Plexams) removePreplanSameSlot(ctx context.Context, id, remove int) error {
	pe, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil || pe == nil || pe.Constraints == nil {
		return err
	}
	kept := make([]int, 0, len(pe.Constraints.SameSlot))
	for _, s := range pe.Constraints.SameSlot {
		if s != remove {
			kept = append(kept, s)
		}
	}
	pe.Constraints.SameSlot = kept
	_, err = p.dbClient.ReplacePreplanExam(ctx, pe)
	return err
}

// preplanConstraintsFromInput maps a ConstraintsInput to a model.Constraints without
// any persistence side effects. SameSlot is kept verbatim (pre-exam ids).
func preplanConstraintsFromInput(input *model.ConstraintsInput) *model.Constraints {
	c := &model.Constraints{}
	if input == nil {
		return c
	}
	if input.NotPlannedByMe != nil {
		c.NotPlannedByMe = *input.NotPlannedByMe
	}
	if input.DoNotPublish != nil {
		c.DoNotPublish = *input.DoNotPublish
	}
	if input.Online != nil {
		c.Online = *input.Online
	}
	c.FixedDay = input.FixedDay
	c.FixedTime = input.FixedTime
	c.ExcludeDays = input.ExcludeDays
	c.PossibleDays = input.PossibleDays
	c.SameSlot = dedupeInts(input.SameSlot)

	rc := &model.RoomConstraints{}
	hasRoom := false
	if len(input.AllowedRooms) > 0 {
		rooms := make([]string, 0, len(input.AllowedRooms))
		for _, r := range input.AllowedRooms {
			if r != "" {
				rooms = append(rooms, r)
			}
		}
		if len(rooms) > 0 {
			rc.AllowedRooms = rooms
			hasRoom = true
		}
	}
	if input.PlacesWithSocket != nil && *input.PlacesWithSocket {
		rc.PlacesWithSocket = true
		hasRoom = true
	}
	if input.Lab != nil && *input.Lab {
		rc.Lab = true
		hasRoom = true
	}
	if input.Exahm != nil && *input.Exahm {
		rc.Exahm = true
		hasRoom = true
	}
	if input.Seb != nil && *input.Seb {
		rc.Seb = true
		hasRoom = true
	}
	if input.MaxStudents != nil && *input.MaxStudents > 0 {
		rc.MaxStudents = input.MaxStudents
		hasRoom = true
	}
	if input.AdditionalSeats != nil && *input.AdditionalSeats > 0 {
		rc.AdditionalSeats = input.AdditionalSeats
		hasRoom = true
	}
	if input.KdpJiraURL != nil && *input.KdpJiraURL != "" {
		rc.KdpJiraURL = input.KdpJiraURL
		hasRoom = true
	}
	if input.Comments != nil && *input.Comments != "" {
		rc.Comments = input.Comments
		hasRoom = true
	}
	if hasRoom {
		c.RoomConstraints = rc
	}
	return c
}

// preplanConstraintsToInput converts a pre-exam's stored constraints into a normal
// ConstraintsInput for AddConstraints, translating the same-slot pre-exam ids into
// the ancodes of those pre-exams that are already linked to a ZPA exam.
func (p *Plexams) preplanConstraintsToInput(ctx context.Context, c *model.Constraints) model.ConstraintsInput {
	in := model.ConstraintsInput{
		NotPlannedByMe: boolPtr(c.NotPlannedByMe),
		DoNotPublish:   boolPtr(c.DoNotPublish),
		Online:         boolPtr(c.Online),
		FixedDay:       c.FixedDay,
		FixedTime:      c.FixedTime,
		ExcludeDays:    c.ExcludeDays,
		PossibleDays:   c.PossibleDays,
	}
	if rc := c.RoomConstraints; rc != nil {
		in.AllowedRooms = rc.AllowedRooms
		in.PlacesWithSocket = boolPtr(rc.PlacesWithSocket)
		in.Lab = boolPtr(rc.Lab)
		in.Exahm = boolPtr(rc.Exahm)
		in.Seb = boolPtr(rc.Seb)
		in.MaxStudents = rc.MaxStudents
		in.AdditionalSeats = rc.AdditionalSeats
		in.KdpJiraURL = rc.KdpJiraURL
		in.Comments = rc.Comments
	}
	ancodes := make([]int, 0, len(c.SameSlot))
	for _, pid := range c.SameSlot {
		other, err := p.dbClient.PreplanExam(ctx, pid)
		if err == nil && other != nil && other.Ancode != nil {
			ancodes = append(ancodes, *other.Ancode)
		}
	}
	in.SameSlot = ancodes
	return in
}

func boolPtr(b bool) *bool { return &b }

func dedupeInts(in []int) []int {
	seen := make(map[int]bool, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func filterPreplanIDs(ids []int, self int, exists map[int]bool) []int {
	out := make([]int, 0, len(ids))
	for _, v := range ids {
		if v != self && exists[v] {
			out = append(out, v)
		}
	}
	return out
}

func intSet(ids []int) map[int]bool {
	s := make(map[int]bool, len(ids))
	for _, v := range ids {
		s[v] = true
	}
	return s
}
