package email

import (
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// RoomRequestEmailTime is one time range a room is occupied.
type RoomRequestEmailTime struct {
	From  string
	Until string
}

// RoomRequestEmailDay groups the time ranges of a room on one day.
type RoomRequestEmailDay struct {
	Date  string
	Times []*RoomRequestEmailTime
}

// RoomRequestEmailRoom groups the days a room is occupied. Shared by the room-request and
// the secretariat-rooms emails (room -> days -> time ranges).
type RoomRequestEmailRoom struct {
	Room string
	Days []*RoomRequestEmailDay
}

// RoomRequestEmail is the data for the Gebäudemanagement room-request email.
type RoomRequestEmail struct {
	SemesterName string
	PlanerName   string
	Rooms        []*RoomRequestEmailRoom
}

// SecretariatRoomsEmail is the data for the rooms-occupancy email to the secretariat: per
// (non-request) room, on which day at which times it is used by an exam.
type SecretariatRoomsEmail struct {
	SemesterName string
	PlanerName   string
	Rooms        []*RoomRequestEmailRoom
}

// lastDay returns the day block for date within room, reusing the last one if it already
// matches (the times are added in ascending order) or appending a new one.
func lastDay(room *RoomRequestEmailRoom, date string) *RoomRequestEmailDay {
	if len(room.Days) > 0 && room.Days[len(room.Days)-1].Date == date {
		return room.Days[len(room.Days)-1]
	}
	day := &RoomRequestEmailDay{Date: date}
	room.Days = append(room.Days, day)
	return day
}

// BuildRoomRequestRooms groups the active room requests by room and then by day, with their
// (buffered) time ranges. Requests are sorted by room, then by start.
func BuildRoomRequestRooms(requests []*model.RoomRequest) []*RoomRequestEmailRoom {
	active := make([]*model.RoomRequest, 0, len(requests))
	for _, req := range requests {
		if req.Active {
			active = append(active, req)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].Room != active[j].Room {
			return active[i].Room < active[j].Room
		}
		return active[i].From.Before(active[j].From)
	})

	rooms := make([]*RoomRequestEmailRoom, 0)
	for _, req := range active {
		if len(rooms) == 0 || rooms[len(rooms)-1].Room != req.Room {
			rooms = append(rooms, &RoomRequestEmailRoom{Room: req.Room})
		}
		room := rooms[len(rooms)-1]
		day := lastDay(room, DateDE(req.From))
		day.Times = append(day.Times, &RoomRequestEmailTime{From: TimeDE(req.From), Until: TimeDE(req.Until)})
	}
	return rooms
}

// roomInterval is one occupancy of a room (start..end).
type roomInterval struct {
	start time.Time
	end   time.Time
}

// mergeRoomIntervals sorts the intervals by start and merges overlapping or touching ones
// into a single range (so the different durations of an exam and its NTAs in the same room
// collapse to one time range).
func mergeRoomIntervals(intervals []roomInterval) []roomInterval {
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start.Before(intervals[j].start)
	})
	merged := []roomInterval{intervals[0]}
	for _, iv := range intervals[1:] {
		last := &merged[len(merged)-1]
		// overlap or touch: iv.start <= last.end
		if !iv.start.After(last.end) {
			if iv.end.After(last.end) {
				last.end = iv.end
			}
			continue
		}
		merged = append(merged, iv)
	}
	return merged
}

// BuildSecretariatRooms lists, per room that does not have to be requested separately, when
// it is occupied by an exam. Overlapping times (e.g. caused by NTAs) are merged. roomInfo
// maps a room name to its master data (a room is skipped when unknown, deactivated, needing
// a request or an ONLINE pseudo-room); slotTime resolves a (day, slot) to its start.
func BuildSecretariatRooms(plannedRooms []*model.PlannedRoom, roomInfo map[string]*model.Room,
	slotTime func(day, slot int) time.Time,
) []*RoomRequestEmailRoom {
	// the online "rooms" are not real bookable rooms for the secretariat.
	skipRoom := map[string]bool{"ONLINE": true, "ONLINE_1": true, "ONLINE_2": true}

	intervalsByRoom := make(map[string][]roomInterval)
	for _, pr := range plannedRooms {
		room, ok := roomInfo[pr.RoomName]
		if !ok || room.NeedsRequest || room.Deactivated || skipRoom[pr.RoomName] {
			continue // only real, active rooms that do not have to be requested
		}
		start := slotTime(pr.Day, pr.Slot)
		end := start.Add(time.Duration(pr.Duration) * time.Minute)
		intervalsByRoom[pr.RoomName] = append(intervalsByRoom[pr.RoomName], roomInterval{start: start, end: end})
	}

	roomNames := make([]string, 0, len(intervalsByRoom))
	for name := range intervalsByRoom {
		roomNames = append(roomNames, name)
	}
	sort.Strings(roomNames)

	rooms := make([]*RoomRequestEmailRoom, 0, len(roomNames))
	for _, name := range roomNames {
		merged := mergeRoomIntervals(intervalsByRoom[name])
		emailRoom := &RoomRequestEmailRoom{Room: name}
		for _, iv := range merged {
			day := lastDay(emailRoom, DateDE(iv.start))
			day.Times = append(day.Times, &RoomRequestEmailTime{From: TimeDE(iv.start), Until: TimeDE(iv.end)})
		}
		rooms = append(rooms, emailRoom)
	}
	return rooms
}
