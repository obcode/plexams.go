package email

import (
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// PublishedRoomsEmail is the data for one examer's "rooms published" email.
type PublishedRoomsEmail struct {
	Teacher    *model.Teacher
	PlanerName string
	Exams      []*PublishedRoomsExam
}

// PublishedRoomsExam is one exam of the examer with its rooms and the co-usage.
type PublishedRoomsExam struct {
	Ancode int
	Module string
	Date   string    // e.g. "Mo, 13.07.2026"
	Time   string    // e.g. "08:30"
	start  time.Time // for chronological sorting (Date is a weekday-prefixed string)
	Rooms  []*PublishedRoomsRoom
}

// PublishedRoomsRoom is one room of an exam with its seat blocks and co-usage.
type PublishedRoomsRoom struct {
	RoomName    string
	Allocations []string                // seat blocks of this room (non-NTA first, then by duration)
	SharedWith  []*PublishedRoomsShared // other exams using the same room in the same slot
}

// PublishedRoomsShared is one other exam sharing a room, with its seat blocks.
type PublishedRoomsShared struct {
	ExamHeader  string   // "324. IT-Sicherheit … (Schreck, Thomas)"
	Allocations []string // that exam's seat blocks in the shared room (non-NTA first, then by duration)
}

// roomNTA returns the NTA assigned to a room (via NtaMtknr), or nil. (A copy of the pure
// plexams helper of the same name; duplicated here so this output package stays decoupled
// from the ics export concern that also uses it.)
func roomNTA(exam *model.PlannedExam, room *model.PlannedRoom) *model.NTA {
	if room.NtaMtknr == nil {
		return nil
	}
	for _, nta := range exam.Ntas {
		if nta != nil && nta.Mtknr == *room.NtaMtknr {
			return nta
		}
	}
	return nil
}

// ntaNote formats the NTA of a room as "Name (+X%[, eigener Raum])", or "".
func ntaNote(exam *model.PlannedExam, room *model.PlannedRoom) string {
	nta := roomNTA(exam, room)
	if nta == nil {
		return ""
	}
	alone := ""
	if nta.NeedsRoomAlone {
		alone = ", eigener Raum"
	}
	return fmt.Sprintf("%s (+%d%%%s)", nta.Name, nta.DeltaDurationPercent, alone)
}

// roomAllocations returns the seat blocks ("N Stud., D min[, NTA: …]") of one exam in one
// room, ordered: non-NTA blocks first, then by duration ascending.
func roomAllocations(exam *model.PlannedExam, roomName string) []string {
	type seatBlock struct {
		hasNTA   bool
		duration int
		text     string
	}
	blocks := make([]seatBlock, 0)
	for _, room := range exam.PlannedRooms {
		if room.RoomName != roomName {
			continue
		}
		text := fmt.Sprintf("%d Stud., %d min", len(room.StudentsInRoom), room.Duration)
		if room.Reserve {
			text += " (Reserve)"
		}
		note := ntaNote(exam, room)
		if note != "" {
			text += ", NTA: " + note
		}
		blocks = append(blocks, seatBlock{hasNTA: note != "", duration: room.Duration, text: text})
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].hasNTA != blocks[j].hasNTA {
			return !blocks[i].hasNTA // non-NTA first
		}
		return blocks[i].duration < blocks[j].duration
	})
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.text
	}
	return texts
}

// BuildPublishedRoomsExam renders one exam for the examer email: per room the exam's own
// seat blocks and, grouped per other exam, the co-usage of that room in the same slot.
// Returns nil if the exam has no plan entry or no real rooms. examsInSlot maps a (day, slot)
// to all exams placed there (pre-fetched by the caller); examerShort resolves an exam to its
// examer's short name; slotTime resolves a plan entry to its start.
func BuildPublishedRoomsExam(exam *model.PlannedExam,
	examsInSlot map[[2]int][]*model.PlannedExam, examerShort func(*model.PlannedExam) string,
	slotTime func(day, slot int) time.Time,
) *PublishedRoomsExam {
	if exam.PlanEntry == nil {
		return nil
	}
	day, slot := exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber
	start := slotTime(day, slot)

	// room names of this exam, first-seen order
	order := make([]string, 0)
	seen := make(map[string]bool)
	for _, room := range exam.PlannedRooms {
		if seen[room.RoomName] {
			continue
		}
		seen[room.RoomName] = true
		order = append(order, room.RoomName)
	}
	if len(order) == 0 {
		return nil
	}

	rooms := make([]*PublishedRoomsRoom, 0, len(order))
	for _, name := range order {
		// co-usage: other exams (grouped) with seat blocks in the same room.
		shared := make([]*PublishedRoomsShared, 0)
		for _, other := range examsInSlot[[2]int{day, slot}] {
			if other.Ancode == exam.Ancode {
				continue
			}
			allocs := roomAllocations(other, name)
			if len(allocs) == 0 {
				continue
			}
			module := ""
			if other.ZpaExam != nil {
				module = other.ZpaExam.Module
			}
			shared = append(shared, &PublishedRoomsShared{
				ExamHeader:  fmt.Sprintf("%d. %s (%s)", other.Ancode, module, examerShort(other)),
				Allocations: allocs,
			})
		}

		rooms = append(rooms, &PublishedRoomsRoom{
			RoomName:    name,
			Allocations: roomAllocations(exam, name),
			SharedWith:  shared,
		})
	}

	module := ""
	if exam.ZpaExam != nil {
		module = exam.ZpaExam.Module
	}
	return &PublishedRoomsExam{
		Ancode: exam.Ancode,
		Module: module,
		Date:   DateDE(start),
		Time:   TimeDE(start),
		start:  start,
		Rooms:  rooms,
	}
}

// SortPublishedRoomsExams orders the exams chronologically by their slot start (stable).
func SortPublishedRoomsExams(exams []*PublishedRoomsExam) {
	sort.SliceStable(exams, func(i, j int) bool {
		return exams[i].start.Before(exams[j].start)
	})
}
