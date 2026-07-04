// Package roomcalc holds the pure room-seat math of the room preparation: whether a
// room satisfies an exam's room constraints (EXaHM/SEB/Lab/socket/allowed-rooms
// feature matching), the required free-seat buffer for an exam, and the free seats an
// exam currently has across its rooms. All functions are I/O-free over graph/model
// types; the stateful room allocation and DB access stay in the plexams package.
package roomcalc

import (
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
)

// FreeSeatsMin / FreeSeatsPercent define the minimum free-seat buffer an exam must keep:
// at least FreeSeatsMin seats and at least FreeSeatsPercent percent of the normal
// students, so no exam is packed exactly full.
const (
	FreeSeatsMin     = 2
	FreeSeatsPercent = 5
)

// FreeSeatsBuffer returns the required free-seat buffer for an exam with the given
// number of normal (non-NTA) students: max(FreeSeatsMin, ceil(FreeSeatsPercent%)).
func FreeSeatsBuffer(normalStudents int) int {
	buf := (normalStudents*FreeSeatsPercent + 99) / 100 // ceil(percent%)
	if buf < FreeSeatsMin {
		buf = FreeSeatsMin
	}
	return buf
}

// ExamFreeSeats returns, for one exam, the free seats across its normal rooms
// (NTA-alone rooms excluded, reserve rooms count as free) and the number of
// students placed in normal rooms.
func ExamFreeSeats(roomInfo map[string]*model.Room, examRooms []*model.PlannedRoom, ancode int) (free, normalStudents int) {
	capacity, reserveSeats := 0, 0
	for _, r := range examRooms {
		if r.Ancode != ancode || r.NtaMtknr != nil {
			continue
		}
		seats := 0
		if room, ok := roomInfo[r.RoomName]; ok {
			seats = room.Seats
		}
		if r.Reserve {
			reserveSeats += seats
			continue
		}
		normalStudents += len(r.StudentsInRoom)
		capacity += seats
	}
	return capacity - normalStudents + reserveSeats, normalStudents
}

// SatisfiesConstraints reports whether a room may host an exam with the given room
// constraints. A room with a special feature (EXaHM / Lab / SEB) is only used for an
// exam that requires at least one feature the room actually has; never for one that
// requires none of them. EXaHM and SEB are compatible: an EXaHM room may also host SEB
// exams (the T-building EXaHM rooms run SEB too). A Lab room only serves Lab exams (or
// SEB/EXaHM if it has those). An all-false RoomConstraints object (present but nothing
// required) must not let such a room slip through.
func SatisfiesConstraints(room *model.Room, constraints *model.Constraints) bool {
	var rc *model.RoomConstraints
	if constraints != nil {
		rc = constraints.RoomConstraints
	}

	if room.Exahm || room.Lab || room.Seb {
		needsFeature := rc != nil && ((rc.Exahm && room.Exahm) ||
			(rc.Seb && (room.Seb || room.Exahm)) ||
			(rc.Lab && room.Lab))
		if !needsFeature {
			return false
		}
	}

	if rc == nil {
		// room without constraints should be no lab!
		return !room.Exahm && !room.Lab && !room.Seb
	}
	if rc.Exahm && !room.Exahm {
		return false
	}
	if rc.Lab && !room.Lab {
		return false
	}
	if rc.PlacesWithSocket && !room.PlacesWithSocket {
		return false
	}
	if rc.Seb && !room.Seb && !room.Exahm { // a SEB exam fits a SEB or an EXaHM room
		return false
	}
	if rc.AllowedRooms != nil && !set.NewSet(rc.AllowedRooms...).Contains(room.Name) {
		return false
	}

	return true
}

// SortPrePlannedRooms orders an exam's pre-planned rooms into fill order, in place: NTA
// rooms (Mtknr != nil) first (keeping their relative input order), then the non-reserve
// rooms by descending seat count, then the reserve rooms last (keeping their relative input
// order). roomInfo provides the seat counts for the non-reserve ordering.
func SortPrePlannedRooms(rooms []*model.PrePlannedRoom, roomInfo map[string]*model.Room) {
	sort.Slice(rooms, func(i, j int) bool {
		// First priority: rooms with Mtknr != nil come first.
		if (rooms[i].Mtknr != nil) != (rooms[j].Mtknr != nil) {
			return rooms[i].Mtknr != nil
		}
		// If both have Mtknr != nil, keep original order.
		if rooms[i].Mtknr != nil && rooms[j].Mtknr != nil {
			return false
		}
		// Last priority: reserve rooms come last.
		if rooms[i].Reserve != rooms[j].Reserve {
			return !rooms[i].Reserve
		}
		// Both non-reserve: sort by seats (descending).
		if !rooms[i].Reserve && !rooms[j].Reserve {
			return roomInfo[rooms[i].RoomName].Seats > roomInfo[rooms[j].RoomName].Seats
		}
		// Keep original order for reserve rooms.
		return false
	})
}

// ExamRegsAndNTAs partitions an exam's registrations for the room allocation: the NTAs are
// split into those needing a room alone and those going into normal rooms; normalRegs holds
// the Mtknrs of all non-NTA student registrations across the exam's Primuss sections, in
// section then registration order.
func ExamRegsAndNTAs(exam *model.PlannedExam) (normalRegs []string, ntasInNormalRooms, ntasInAloneRooms []*model.NTA) {
	ntaMtknrs := set.NewSet[string]()
	ntasInNormalRooms = make([]*model.NTA, 0)
	ntasInAloneRooms = make([]*model.NTA, 0)
	for _, nta := range exam.Ntas {
		ntaMtknrs.Add(nta.Mtknr)
		if nta.NeedsRoomAlone {
			ntasInAloneRooms = append(ntasInAloneRooms, nta)
		} else {
			ntasInNormalRooms = append(ntasInNormalRooms, nta)
		}
	}

	normalRegs = make([]string, 0)
	for _, primussExam := range exam.PrimussExams {
		for _, studentReg := range primussExam.StudentRegs {
			if !ntaMtknrs.Contains(studentReg.Mtknr) {
				normalRegs = append(normalRegs, studentReg.Mtknr)
			}
		}
	}
	return normalRegs, ntasInNormalRooms, ntasInAloneRooms
}
