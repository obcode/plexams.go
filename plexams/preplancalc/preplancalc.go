// Package preplancalc holds the pure room-capacity math of the SEB/EXaHM pre-planning:
// which rooms cover a slot's seat demand for a kind (honouring per-exam room restrictions),
// how many seats a room set provides, which rooms still need to be booked, and the
// per-program clashes within a slot. All functions are I/O-free over graph/model types and
// the RoomCapacity value; the DB access and orchestration stay in the plexams package.
package preplancalc

import (
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// RoomCapacity is one room usable for a given kind, with its seat count.
type RoomCapacity struct {
	Name  string
	Seats int
}

// NormRoomName upper-cases a room name and strips spaces, so "t 3.014" and "T3.014"
// compare equal.
func NormRoomName(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// TotalSeats sums the seats of the given rooms.
func TotalSeats(rooms []RoomCapacity) int {
	total := 0
	for _, r := range rooms {
		total += r.Seats
	}
	return total
}

// RoomsToBook greedily picks rooms (largest first) that are NOT yet booked for the slot,
// enough to cover the still-missing seats (gap). Empty when nothing is missing.
func RoomsToBook(rooms []RoomCapacity, gap int, booked map[string]bool) []string {
	names := make([]string, 0)
	remaining := gap
	for _, r := range rooms {
		if remaining <= 0 {
			break
		}
		if booked != nil && booked[NormRoomName(r.Name)] {
			continue
		}
		names = append(names, r.Name)
		remaining -= r.Seats
	}
	return names
}

// RoomsForKind restricts the candidate rooms by the per-exam allowedRooms of the slot's
// exams of that kind: only when every such exam restricts its rooms is the pool narrowed to
// the union of their allowedRooms (an exam without a restriction may use any room, so the
// full set is kept).
func RoomsForKind(exams []*model.PreplanExam, kind string, rooms []RoomCapacity) []RoomCapacity {
	allowed := make(map[string]bool)
	hasRestriction, hasUnrestricted := false, false
	for _, pe := range exams {
		if pe.ExamKind != kind {
			continue
		}
		var ar []string
		if pe.Constraints != nil && pe.Constraints.RoomConstraints != nil {
			ar = pe.Constraints.RoomConstraints.AllowedRooms
		}
		if len(ar) == 0 {
			hasUnrestricted = true
			continue
		}
		hasRestriction = true
		for _, r := range ar {
			allowed[NormRoomName(r)] = true
		}
	}
	if !hasRestriction || hasUnrestricted {
		return rooms
	}
	filtered := make([]RoomCapacity, 0, len(rooms))
	for _, r := range rooms {
		if allowed[NormRoomName(r.Name)] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// KindNeed sums the seat demand of the pre-exams of one kind in a slot and greedily picks
// rooms (largest first) to cover it, honouring per-exam room restrictions.
func KindNeed(exams []*model.PreplanExam, kind string, rooms []RoomCapacity) *model.PreplanKindNeed {
	count, seats := 0, 0
	for _, pe := range exams {
		if pe.ExamKind == kind {
			count++
			seats += pe.ExpectedStudents
		}
	}

	pool := RoomsForKind(exams, kind, rooms)
	available := TotalSeats(pool)

	roomNames := make([]string, 0)
	remaining := seats
	for _, r := range pool {
		if remaining <= 0 {
			break
		}
		roomNames = append(roomNames, r.Name)
		remaining -= r.Seats
	}

	return &model.PreplanKindNeed{
		ExamCount:      count,
		SeatsNeeded:    seats,
		RoomsSuggested: len(roomNames),
		Rooms:          roomNames,
		SeatsAvailable: available,
		SeatsBooked:    0,
		RoomsToBook:    []string{},
	}
}

// ApplyBooking fills the booked seats and the still-to-book rooms for one kind.
func ApplyBooking(need *model.PreplanKindNeed, bookedSeats int, rooms []RoomCapacity, bookedRooms map[string]bool) {
	need.SeatsBooked = bookedSeats
	gap := need.SeatsNeeded - bookedSeats
	if gap < 0 {
		gap = 0
	}
	need.RoomsToBook = RoomsToBook(rooms, gap, bookedRooms)
}

// ProgramConflicts finds study programs that appear in more than one pre-exam of the same
// slot (a possible student clash, since Primuss conflicts aren't known yet).
func ProgramConflicts(exams []*model.PreplanExam) []*model.PreplanProgramConflict {
	type acc struct {
		ids     []int
		modules []string
	}
	byProgram := make(map[string]*acc)
	order := make([]string, 0)
	for _, pe := range exams {
		for _, prog := range pe.Programs {
			a, ok := byProgram[prog]
			if !ok {
				a = &acc{}
				byProgram[prog] = a
				order = append(order, prog)
			}
			a.ids = append(a.ids, pe.ID)
			a.modules = append(a.modules, pe.Module)
		}
	}

	conflicts := make([]*model.PreplanProgramConflict, 0)
	sort.Strings(order)
	for _, prog := range order {
		a := byProgram[prog]
		if len(a.ids) > 1 {
			conflicts = append(conflicts, &model.PreplanProgramConflict{
				Program:        prog,
				PreplanExamIDs: a.ids,
				Modules:        a.modules,
			})
		}
	}
	return conflicts
}
