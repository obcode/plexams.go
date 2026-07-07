package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// normalRoomSeats is the assumed capacity of an ordinary room; an exam with at
// most this many students does not need a building-management request room.
const normalRoomSeats = 25

// roomRequestBuffer is how long a booked room (Gebäudemanagement / anny) must be
// available before an exam starts (setup) and after it ends (teardown). A single NTA's
// time extension runs into the post-buffer (e.g. a 9-minute extension leaves 6 minutes),
// which is enough for individual NTAs; longer extensions get their own room later — not a
// scheduling concern. Applied on both ends of the room window around the BASE exam
// duration (NTAs are not added on top).
const roomRequestBuffer = 15 * time.Minute

// managementRooms returns the active rooms that are requested via the building
// management.
func (p *Plexams) managementRooms(ctx context.Context) ([]*model.Room, error) {
	rooms, err := p.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	mgmt := make([]*model.Room, 0)
	for _, room := range rooms {
		if room.Deactivated {
			continue
		}
		if room.RequestWith == model.RoomRequestTypeManagement {
			mgmt = append(mgmt, room)
		}
	}
	return mgmt, nil
}

// coverWithin tries to seat studs students with rooms from pool: a single
// smallest-fitting room if one exists (tie: lower priority), otherwise the rooms
// of the pool added in priority order (then by seats desc) until they cover
// studs. Returns nil if the pool cannot cover studs.
func coverWithin(pool []*model.Room, studs int) []*model.Room {
	var best *model.Room
	for _, r := range pool {
		if r.Seats >= studs {
			if best == nil || r.Seats < best.Seats ||
				(r.Seats == best.Seats && r.RequestPriority < best.RequestPriority) {
				best = r
			}
		}
	}
	if best != nil {
		return []*model.Room{best}
	}

	sorted := make([]*model.Room, len(pool))
	copy(sorted, pool)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].RequestPriority != sorted[j].RequestPriority {
			return sorted[i].RequestPriority < sorted[j].RequestPriority
		}
		return sorted[i].Seats > sorted[j].Seats
	})

	chosen := make([]*model.Room, 0, len(sorted))
	sum := 0
	for _, r := range sorted {
		chosen = append(chosen, r)
		sum += r.Seats
		if sum >= studs {
			return chosen
		}
	}
	return nil
}

// pickRequestRooms chooses the management rooms for one exam: it widens the pool
// by priority (preferred rooms first) and only falls back to lower-priority rooms
// when the preferred ones cannot seat the exam. Rooms already used in the slot
// are excluded. If no combination covers studs, all free rooms are returned as a
// best effort (the under-capacity is then visible in the preview).
func pickRequestRooms(mgmt []*model.Room, usedInSlot map[string]bool, studs int) []*model.Room {
	free := make([]*model.Room, 0, len(mgmt))
	for _, room := range mgmt {
		if !usedInSlot[room.Name] {
			free = append(free, room)
		}
	}

	priorities := make([]int, 0)
	seen := make(map[int]bool)
	for _, room := range free {
		if !seen[room.RequestPriority] {
			seen[room.RequestPriority] = true
			priorities = append(priorities, room.RequestPriority)
		}
	}
	sort.Ints(priorities)

	pool := make([]*model.Room, 0, len(free))
	for _, prio := range priorities {
		for _, room := range free {
			if room.RequestPriority == prio {
				pool = append(pool, room)
			}
		}
		if chosen := coverWithin(pool, studs); chosen != nil {
			return chosen
		}
	}
	return free
}

// needsRequestRoom reports whether an exam is a candidate for a building-
// management request room: planned by me and without specific room constraints
// (exahm/lab/seb/places-with-socket are handled elsewhere).
func needsRequestRoom(exam *model.PlannedExam) bool {
	if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
		return false
	}
	if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil {
		rc := exam.Constraints.RoomConstraints
		if rc.Exahm || rc.Lab || rc.Seb || rc.PlacesWithSocket {
			return false
		}
	}
	return true
}

// GenerateRoomRequestsPreview computes, per slot, which active management rooms
// would be requested for which exams (preferring higher-priority rooms). It is a
// read-only dry run and changes nothing. Each entry carries the triggering exam
// and the other exams in the same slot.
func (p *Plexams) GenerateRoomRequestsPreview(ctx context.Context) ([]*model.RoomRequestPreview, error) {
	mgmt, err := p.managementRooms(ctx)
	if err != nil {
		return nil, err
	}

	preview := make([]*model.RoomRequestPreview, 0)
	for _, slot := range p.semesterConfig.Slots {
		examsInSlot, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			return nil, err
		}

		candidates := make([]*model.PlannedExam, 0, len(examsInSlot))
		for _, exam := range examsInSlot {
			if needsRequestRoom(exam) {
				candidates = append(candidates, exam)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].StudentRegsCount > candidates[j].StudentRegsCount
		})

		start := p.getSlotTime(slot.DayNumber, slot.SlotNumber)
		usedInSlot := make(map[string]bool)
		for _, exam := range candidates {
			studs := exam.StudentRegsCount
			if studs <= normalRoomSeats {
				break // candidates are sorted desc, so the rest is small too
			}
			chosen := pickRequestRooms(mgmt, usedInSlot, studs)
			if len(chosen) == 0 {
				continue
			}
			// Room window = [start - preBuffer, start + base duration + postBuffer]. A single
			// NTA runs into the post-buffer; larger extensions get a separate room later. The
			// buffers default to 15 min but an exam may request a larger Vorlauf/Nachlauf.
			duration := 0
			if exam.ZpaExam != nil {
				duration = exam.ZpaExam.Duration
			}
			preBuffer, postBuffer := roomBuffers(exam.Constraints)
			from := start.Add(-preBuffer)
			until := start.Add(time.Duration(duration)*time.Minute + postBuffer)
			simultaneous := make([]*model.PlannedExam, 0, len(examsInSlot))
			for _, other := range examsInSlot {
				if other.Ancode != exam.Ancode {
					simultaneous = append(simultaneous, other)
				}
			}
			for _, room := range chosen {
				usedInSlot[room.Name] = true
				preview = append(preview, &model.RoomRequestPreview{
					Room:              room.Name,
					Day:               slot.DayNumber,
					Slot:              slot.SlotNumber,
					From:              from,
					Until:             until,
					Students:          studs,
					Seats:             room.Seats,
					Exam:              exam,
					SimultaneousExams: simultaneous,
				})
			}
		}
	}

	sort.SliceStable(preview, func(i, j int) bool {
		if preview[i].Room != preview[j].Room {
			return preview[i].Room < preview[j].Room
		}
		if preview[i].Day != preview[j].Day {
			return preview[i].Day < preview[j].Day
		}
		return preview[i].Slot < preview[j].Slot
	})
	return preview, nil
}
