package anny

import (
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// RoomBooking is one or more rooms booked in Anny for a time window, with the approval
// status. It is the merged, planning-facing view (see MergeRoomBookings) of the raw Anny
// bookings; it feeds the EXaHM room availability per slot.
type RoomBooking struct {
	From     time.Time
	Until    time.Time
	Rooms    []string
	Approved bool
}

// normRoom normalizes a room name for comparison (upper-cased, spaces removed).
func normRoom(room string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(room), " ", ""))
}

// MergeRoomBookings sorts the bookings by room, approval and time, and merges adjacent or
// overlapping bookings of the same single room and approval status into one window.
// Bookings that do not name exactly one room are left untouched.
func MergeRoomBookings(entries []RoomBooking) []RoomBooking {
	if len(entries) < 2 {
		return entries
	}

	sortedEntries := make([]RoomBooking, len(entries))
	copy(sortedEntries, entries)

	sort.Slice(sortedEntries, func(i, j int) bool {
		roomI := ""
		if len(sortedEntries[i].Rooms) > 0 {
			roomI = normRoom(sortedEntries[i].Rooms[0])
		}
		roomJ := ""
		if len(sortedEntries[j].Rooms) > 0 {
			roomJ = normRoom(sortedEntries[j].Rooms[0])
		}

		if roomI != roomJ {
			return roomI < roomJ
		}
		if sortedEntries[i].Approved != sortedEntries[j].Approved {
			return sortedEntries[i].Approved && !sortedEntries[j].Approved
		}
		if !sortedEntries[i].From.Equal(sortedEntries[j].From) {
			return sortedEntries[i].From.Before(sortedEntries[j].From)
		}
		return sortedEntries[i].Until.Before(sortedEntries[j].Until)
	})

	merged := make([]RoomBooking, 0, len(sortedEntries))
	for _, current := range sortedEntries {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}

		last := &merged[len(merged)-1]
		if len(last.Rooms) != 1 || len(current.Rooms) != 1 {
			merged = append(merged, current)
			continue
		}

		// Merge adjacent or overlapping bookings for the same room and approval status.
		if normRoom(last.Rooms[0]) == normRoom(current.Rooms[0]) &&
			last.Approved == current.Approved &&
			(current.From.Before(last.Until) || current.From.Equal(last.Until)) {
			if current.Until.After(last.Until) {
				last.Until = current.Until
			}
			continue
		}

		merged = append(merged, current)
	}

	return merged
}

// RoomBookedDuringExamTime reports whether any booking fully covers the exam window of the
// slot — from the slot start to 90 minutes later, with the boundaries inclusive (a booking
// starting/ending exactly at an exam boundary counts).
func RoomBookedDuringExamTime(bookings []RoomBooking, slot *model.Slot) bool {
	if slot == nil {
		return false
	}
	examStart := slot.Starttime
	examEnd := slot.Starttime.Add(90 * time.Minute)
	for _, booking := range bookings {
		if !booking.From.After(examStart) && !booking.Until.Before(examEnd) {
			return true
		}
	}
	return false
}

// CoversSlot reports whether a booking spanning [from, until) makes its rooms usable in an
// exam slot starting at slotStart: the booking must start before the slot and end after
// the slot's ~90-minute window (slotStart + 89 minutes).
func CoversSlot(from, until, slotStart time.Time) bool {
	return from.Before(slotStart) && until.After(slotStart.Add(89*time.Minute))
}
