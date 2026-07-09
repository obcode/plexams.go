package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/obcode/plexams.go/plexams/preplancalc"
)

// exahmDefaultBuffer is the default setup (Vorlauf) / teardown (Nachlauf) time an
// EXaHM/SEB exam needs its booked T-building room free around it — against a booking edge
// or another (foreign) exam. Between two of OUR OWN consecutive exams in the same room the
// same 30 min is SHARED (each side contributes half, i.e. 15), so a room booked 14:00–18:30
// fits two 90-min exams at 14:30 and 16:30 (30 to each edge, 30 gap between them).
//
// Overridable per exam via RoomConstraints.PreExamMinutes / PostExamMinutes: a lab exam
// with heavy setup may require e.g. a real 60 min each side (then NOT shared), and a light
// exam may shorten it (e.g. 15). See exahmRoomBuffers.
const exahmDefaultBuffer = 30 * time.Minute

// exahmRoomBuffers returns the setup (pre) / teardown (post) time an EXaHM/SEB exam needs
// its booked room free around it: exahmDefaultBuffer (30 min) unless the exam's
// RoomConstraints override it via PreExamMinutes / PostExamMinutes. Unlike roomBuffers
// (regular Gebäudemanagement rooms, widen-only), an EXaHM override REPLACES the default and
// may also shorten it — but never below 1 min (a 0/negative value is ignored).
func exahmRoomBuffers(constraints *model.Constraints) (pre, post time.Duration) {
	pre, post = exahmDefaultBuffer, exahmDefaultBuffer
	if constraints == nil || constraints.RoomConstraints == nil {
		return pre, post
	}
	rc := constraints.RoomConstraints
	if rc.PreExamMinutes != nil && *rc.PreExamMinutes > 0 {
		pre = time.Duration(*rc.PreExamMinutes) * time.Minute
	}
	if rc.PostExamMinutes != nil && *rc.PostExamMinutes > 0 {
		post = time.Duration(*rc.PostExamMinutes) * time.Minute
	}
	return pre, post
}

// bookedRoomInterval is one of our booked T-building rooms as a time interval, together
// with the room's EXaHM/SEB capability and physical seats. Overlapping/adjacent bookings of
// the same room are merged (see bookedExahmIntervals).
type bookedRoomInterval struct {
	from, until time.Time
	exahm, seb  bool
	seats       int // EXaHM / physical seats
	sebSeats    int // SEB seats (room.SebSeats override, else physical)
}

// bookedExahmIntervals returns our booked T-building room time intervals (merged per room)
// with each room's EXaHM/SEB capability. Only our own bookings count (matched by the
// configured personalization names) in rooms flagged RequestWith == ANNY. It is the
// time-interval view of the same bookings annyBookedByTime turns into per-slot seat counts,
// used to check a placement against the REAL booking window (not just a fixed slot block).
func (p *Plexams) bookedExahmIntervals(ctx context.Context) ([]bookedRoomInterval, error) {
	bookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}
	names := p.anny.PersonalizationNames(ctx)

	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	annyRoom := make(map[string]*model.Room, len(rooms))
	for _, r := range rooms {
		if !r.Deactivated && r.RequestWith == model.RoomRequestTypeAnny {
			annyRoom[preplancalc.NormRoomName(r.Name)] = r
		}
	}

	entries := make([]anny.RoomBooking, 0, len(bookings))
	for _, b := range bookings {
		if b.Room == "" || !anny.MatchesAnyPersonalization(b.PersonalizationName, names) {
			continue
		}
		if !anny.IsApprovedStatus(b.Status) {
			continue // only confirmed bookings count as real room capacity
		}
		if _, ok := annyRoom[preplancalc.NormRoomName(b.Room)]; !ok {
			continue
		}
		entries = append(entries, anny.RoomBooking{
			From:     b.StartDate,
			Until:    b.EndDate,
			Rooms:    []string{b.Room},
			Approved: anny.IsApprovedStatus(b.Status),
		})
	}

	merged := anny.MergeRoomBookings(entries)
	result := make([]bookedRoomInterval, 0, len(merged))
	for _, m := range merged {
		if len(m.Rooms) != 1 {
			continue
		}
		room := annyRoom[preplancalc.NormRoomName(m.Rooms[0])]
		if room == nil {
			continue
		}
		sebSeats := room.Seats
		if room.SebSeats != nil {
			sebSeats = *room.SebSeats
		}
		result = append(result, bookedRoomInterval{
			from: m.From, until: m.Until,
			exahm: room.Exahm, seb: room.Seb, seats: room.Seats, sebSeats: sebSeats,
		})
	}
	return result, nil
}

// preplanExamDuration returns a pre-exam's exam duration, falling back to a full slot
// block when none has been entered yet (so an un-sized pre-exam is gated conservatively).
func preplanExamDuration(pe *model.PreplanExam, fallback time.Duration) time.Duration {
	if pe.Duration != nil && *pe.Duration > 0 {
		return time.Duration(*pe.Duration) * time.Minute
	}
	return fallback
}

// intersectSlotSet intersects two slot-index allow-sets, treating nil as "no restriction"
// (any slot). The result is a fresh map (never one of the inputs), so callers may safely
// share the inputs (e.g. the reused MUC.DAI slot set).
func intersectSlotSet(a, b map[int]bool) map[int]bool {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := make(map[int]bool, len(a))
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

// exahmOccBuffers returns the room-OCCUPANCY buffers used for the capacity-over-time check
// (how long an exam keeps its rooms busy for setup/teardown). A DEFAULT buffer is SHARED
// between two of our own consecutive exams, so only half of it (15 min) counts as occupancy
// and back-to-back slots abut; an OVERRIDDEN buffer (e.g. Embedded Computing's 60 min) is a
// real, exclusive requirement and counts in full — its setup then reaches back into the
// previous slot. Booking-coverage still uses the full buffers (exahmRoomBuffers).
func exahmOccBuffers(constraints *model.Constraints) (pre, post time.Duration) {
	pre, post = exahmDefaultBuffer/2, exahmDefaultBuffer/2
	if constraints == nil || constraints.RoomConstraints == nil {
		return pre, post
	}
	rc := constraints.RoomConstraints
	if rc.PreExamMinutes != nil && *rc.PreExamMinutes > 0 {
		pre = time.Duration(*rc.PreExamMinutes) * time.Minute
	}
	if rc.PostExamMinutes != nil && *rc.PostExamMinutes > 0 {
		post = time.Duration(*rc.PostExamMinutes) * time.Minute
	}
	return pre, post
}

// cumExam is one placed exam for the capacity-over-time check: its seat demand, kind and the
// absolute occupancy window [from, to] (exam time plus the occupancy buffers).
type cumExam struct {
	id       int
	seats    int
	exahm    bool
	from, to time.Time
}

// cumOverload records that, during [from, to), the simultaneous demand of a kind exceeded the
// booked seats available then. examIDs are the exams occupying rooms in that interval.
type cumOverload struct {
	exahm         bool
	from, to      time.Time
	demand, seats int
	examIDs       []int
}

// cumulativeOverloads reports every time interval where the simultaneously-occupied EXaHM (or
// total) seats exceed the booked seats available then — even across slots, so a long exam
// whose setup reaches back into the previous slot (Embedded Computing needs its rooms from
// 09:30 while the 08:30 exams still hold them until 10:15) is caught. Rooms may be shared, so
// this is an aggregate seat count over time, not one exam per room. stopAtFirst returns after
// the first overload (for the solver's yes/no feasibility). Raw per-interval records; the
// caller may group them for reporting.
func cumulativeOverloads(exams []cumExam, intervals []bookedRoomInterval, stopAtFirst bool) []cumOverload {
	if len(exams) == 0 || len(intervals) == 0 {
		return nil
	}
	seen := make(map[int64]time.Time)
	add := func(t time.Time) { seen[t.UnixNano()] = t }
	for _, e := range exams {
		add(e.from)
		add(e.to)
	}
	for _, iv := range intervals {
		add(iv.from)
		add(iv.until)
	}
	times := make([]time.Time, 0, len(seen))
	for _, t := range seen {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	var out []cumOverload
	for i := 0; i+1 < len(times); i++ {
		mid := times[i].Add(times[i+1].Sub(times[i]) / 2)
		exahmSeats, totalSeats := 0, 0
		for _, iv := range intervals {
			if iv.from.After(mid) || !iv.until.After(mid) {
				continue
			}
			if iv.exahm {
				exahmSeats += iv.seats
				totalSeats += iv.seats
			} else if iv.seb {
				totalSeats += iv.sebSeats
			}
		}
		exahmDem, totalDem := 0, 0
		var ids []int
		for _, e := range exams {
			if e.from.After(mid) || !e.to.After(mid) {
				continue
			}
			ids = append(ids, e.id)
			if e.exahm {
				exahmDem += e.seats
			}
			totalDem += e.seats
		}
		switch {
		case exahmDem > exahmSeats:
			out = append(out, cumOverload{true, times[i], times[i+1], exahmDem, exahmSeats, ids})
		case totalDem > totalSeats:
			out = append(out, cumOverload{false, times[i], times[i+1], totalDem, totalSeats, ids})
		default:
			continue
		}
		if stopAtFirst {
			return out
		}
	}
	return out
}

// exahmWindowSeats returns how many booked seats are usable for an exam placed at start:
// the sum of seats of booked rooms of the required kind whose Anny window fully covers the
// exam window [start-pre, start+dur+post]. Rooms booked too short (not covering the whole
// window) contribute nothing — so an exam can only be seated by rooms that are actually
// available for its full run plus setup/teardown. exahm selects the kind: EXaHM exams need
// EXaHM-capable rooms; SEB exams accept EXaHM or SEB rooms (SEB may also overflow into
// non-booked R-rooms, so SEB is only gated where a booking is required by the caller).
func exahmWindowSeats(intervals []bookedRoomInterval, exahm bool, start time.Time, dur, pre, post time.Duration) int {
	winStart := start.Add(-pre)
	winEnd := start.Add(dur + post)
	seats := 0
	for _, iv := range intervals {
		if !anny.Covers(iv.from, iv.until, winStart, winEnd) {
			continue
		}
		switch {
		case exahm:
			if iv.exahm {
				seats += iv.seats // EXaHM exams need an EXaHM-capable room
			}
		case iv.seb:
			seats += iv.sebSeats // SEB exam in a SEB room: SEB capacity
		case iv.exahm:
			seats += iv.seats // SEB exam may also use an EXaHM room
		}
	}
	return seats
}

// exahmWindowCovered reports whether some booked room of the required kind fully covers the
// exam window (a placement is possible at all). exahmWindowSeats additionally tells how many
// seats that covering booking provides.
func exahmWindowCovered(intervals []bookedRoomInterval, exahm bool, start time.Time, dur, pre, post time.Duration) bool {
	return exahmWindowSeats(intervals, exahm, start, dur, pre, post) > 0
}
