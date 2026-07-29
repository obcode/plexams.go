package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/obcode/plexams.go/plexams/preplancalc"
)

// The booking proposal answers "where should I still book?" — the counterpart to the
// assignment, which can only distribute exams over what is ALREADY booked. It runs the very
// same solver, but with the capacity of every Anny room that is still FREE (i.e. not taken
// by a foreign booking; our own bookings do not block us). From the resulting hypothetical
// assignment it derives, per slot, the rooms that would have to be booked — minus the ones
// we hold already. Nothing is persisted: neither the pre-exams nor Anny are touched.

// timeWindow is a half-open time interval used while cutting foreign bookings out of a day.
type timeWindow struct {
	from, until time.Time
}

// subtractWindows returns the parts of span not covered by any blocked window. blocked need
// not be sorted or disjoint; empty/inverted results are dropped.
func subtractWindows(span timeWindow, blocked []timeWindow) []timeWindow {
	free := []timeWindow{span}
	sorted := append([]timeWindow(nil), blocked...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].from.Before(sorted[j].from) })
	for _, b := range sorted {
		next := make([]timeWindow, 0, len(free)+1)
		for _, f := range free {
			// no overlap → keep as is
			if !b.from.Before(f.until) || !f.from.Before(b.until) {
				next = append(next, f)
				continue
			}
			if f.from.Before(b.from) {
				next = append(next, timeWindow{f.from, b.from})
			}
			if b.until.Before(f.until) {
				next = append(next, timeWindow{b.until, f.until})
			}
		}
		free = next
	}
	return free
}

// freeAnnyIntervals returns, per Anny (T-building) room and exam day, the windows in which
// the room is NOT blocked by a FOREIGN booking — what we could still book. Our own bookings
// are not subtracted: a room we hold is available to us, and beyond our window it may still
// be extendable. Days are the calendar days of the configured slots, so the proposal never
// suggests a booking outside the exam period.
func (p *Plexams) freeAnnyIntervals(ctx context.Context) ([]bookedRoomInterval, error) {
	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	annyRooms := make([]*model.Room, 0)
	for _, r := range rooms {
		if !r.Deactivated && r.RequestWith == model.RoomRequestTypeAnny && (r.Exahm || r.Seb) {
			annyRooms = append(annyRooms, r)
		}
	}
	if len(annyRooms) == 0 {
		return nil, nil
	}

	bookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}
	names := p.anny.PersonalizationNames(ctx)
	// foreign, confirmed bookings per (normalized) room — they are what blocks us.
	foreign := make(map[string][]timeWindow)
	for _, b := range bookings {
		if b.Room == "" || anny.MatchesAnyPersonalization(b.PersonalizationName, names) {
			continue // ours (or nothing configured → everything counts as ours)
		}
		if !anny.IsApprovedStatus(b.Status) || b.CanceledAt != nil {
			continue
		}
		n := preplancalc.NormRoomName(b.Room)
		foreign[n] = append(foreign[n], timeWindow{b.StartDate, b.EndDate})
	}

	// exam days = the calendar days of the configured slots
	dayOrder := make([]time.Time, 0)
	seen := make(map[string]bool)
	for _, s := range p.semesterConfig.Slots {
		y, m, d := s.Starttime.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, s.Starttime.Location())
		if key := day.Format("2006-01-02"); !seen[key] {
			seen[key] = true
			dayOrder = append(dayOrder, day)
		}
	}

	result := make([]bookedRoomInterval, 0, len(annyRooms)*len(dayOrder))
	for _, room := range annyRooms {
		blocked := foreign[preplancalc.NormRoomName(room.Name)]
		sebSeats := room.Seats
		if room.SebSeats != nil {
			sebSeats = *room.SebSeats
		}
		for _, day := range dayOrder {
			span := timeWindow{day, day.AddDate(0, 0, 1)}
			for _, w := range subtractWindows(span, blocked) {
				result = append(result, bookedRoomInterval{
					room: room.Name, from: w.from, until: w.until,
					exahm: room.Exahm, seb: room.Seb, seats: room.Seats, sebSeats: sebSeats,
				})
			}
		}
	}
	return result, nil
}

// slotSeatsFromIntervals sums the physical seats of the rooms whose window fully covers the
// slot block [start, start+block] — the interval pendant of slotBooking.seats.
func slotSeatsFromIntervals(intervals []bookedRoomInterval, start time.Time, block time.Duration) int {
	seats := 0
	for _, iv := range intervals {
		if anny.Covers(iv.from, iv.until, start, start.Add(block)) {
			seats += iv.seats
		}
	}
	return seats
}

// PreplanBookingSuggestions proposes the Anny bookings still missing so that every pre-exam
// can be planned. With keepAssigned the already-slotted exams keep their slot (so the
// proposal is the MINIMAL addition to what is booked); without it everything is re-planned
// over the free rooms, which is how a proposal can be made before anything is booked.
// Nothing is written.
func (p *Plexams) PreplanBookingSuggestions(ctx context.Context, keepAssigned bool) (*model.PreplanBookingProposal, error) {
	preExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	proposal := &model.PreplanBookingProposal{
		Suggestions:      []*model.PreplanBookingSuggestion{},
		NewlyPlaced:      []*model.PreplanPlacement{},
		StillUnplacedIDs: []int{},
		Findings:         []*model.PreplanFinding{},
	}
	if len(preExams) == 0 {
		proposal.Findings = append(proposal.Findings, &model.PreplanFinding{
			Level:   model.ValidationLevelInfo,
			Message: "keine SEB/EXaHM-Vorplanungsprüfungen vorhanden — nichts zu buchen",
		})
		return proposal, nil
	}
	for _, pe := range preExams {
		if pe.PlannedStarttime == nil {
			proposal.UnplacedNow++
		}
	}

	free, err := p.freeAnnyIntervals(ctx)
	if err != nil {
		return nil, err
	}
	own, err := p.bookedExahmIntervals(ctx)
	if err != nil {
		return nil, err
	}
	rBauSebThreshold, err := p.nonAnnySebSeats(ctx)
	if err != nil {
		return nil, err
	}
	blockDur := slotBlockDuration(p.semesterConfig.Starttimes)

	if len(free) == 0 {
		proposal.Findings = append(proposal.Findings, &model.PreplanFinding{
			Level:   model.ValidationLevelError,
			Message: "kein Anny-Raum ist im Prüfungszeitraum noch frei — es kann nichts vorgeschlagen werden",
		})
		return proposal, nil
	}

	// Capacity = every still-free Anny room. Slots we already hold bookings for are marked,
	// so the solver prefers them (using them needs no new booking) over an equally good slot
	// that would have to be booked first.
	seatsByStart := make(map[time.Time]int)
	alreadyBooked := make(map[time.Time]bool)
	for _, s := range p.semesterConfig.Slots {
		seatsByStart[s.Starttime] = slotSeatsFromIntervals(free, s.Starttime, blockDur)
		alreadyBooked[s.Starttime] = slotSeatsFromIntervals(own, s.Starttime, blockDur) > 0
	}

	plan, err := p.solvePreplanAssignment(ctx, preExams, keepAssigned,
		preplanCapacity{seatsByStart: seatsByStart, intervals: free, alreadyBooked: alreadyBooked},
		rBauSebThreshold, blockDur)
	if err != nil {
		return nil, err
	}

	// exams per proposed slot, in slot order
	bySlot := make(map[time.Time][]*model.PreplanExam)
	slotOrder := make([]time.Time, 0)
	for i, pe := range preExams {
		ps := plan.finalSlot[i]
		if ps == nil {
			proposal.StillUnplacedIDs = append(proposal.StillUnplacedIDs, pe.ID)
			continue
		}
		if pe.PlannedStarttime == nil {
			start := ps.start
			proposal.NewlyPlaced = append(proposal.NewlyPlaced, &model.PreplanPlacement{
				ID: pe.ID, Module: pe.Module, ExamKind: pe.ExamKind,
				ExpectedStudents: pe.ExpectedStudents, Starttime: start,
			})
		}
		if _, ok := bySlot[ps.start]; !ok {
			slotOrder = append(slotOrder, ps.start)
		}
		bySlot[ps.start] = append(bySlot[ps.start], pe)
	}
	sort.Slice(slotOrder, func(i, j int) bool { return slotOrder[i].Before(slotOrder[j]) })

	raw := make([]*model.PreplanBookingSuggestion, 0)
	for _, start := range slotOrder {
		raw = append(raw, roomsToBookForSlot(start, bySlot[start], free, own, blockDur, rBauSebThreshold)...)
	}
	proposal.Suggestions = mergeBookingSuggestions(raw)
	proposal.Findings = append(proposal.Findings, bookingProposalFindings(proposal, preExams, keepAssigned)...)
	return proposal, nil
}

// roomsToBookForSlot picks the rooms one slot needs and returns those we do not hold yet.
// The window is the union of the exam windows (duration plus setup/teardown buffers) of the
// exams placed there, so one booking serves them all. EXaHM demand may only be covered by
// EXaHM-capable rooms; the remaining (SEB) demand by any Anny room. Rooms we have already
// booked for the whole window are counted first — they cost nothing.
func roomsToBookForSlot(start time.Time, exams []*model.PreplanExam, free, own []bookedRoomInterval,
	blockDur time.Duration, rBauSebThreshold int,
) []*model.PreplanBookingSuggestion {
	if len(exams) == 0 {
		return nil
	}
	winFrom, winUntil := time.Time{}, time.Time{}
	exahmDemand, totalDemand := 0, 0
	modules := make([]string, 0, len(exams))
	kinds := make([]string, 0, 2)
	for _, pe := range exams {
		dur := preplanExamDuration(pe, blockDur)
		pre, post := exahmRoomBuffers(pe.Constraints)
		from, until := start.Add(-pre), start.Add(dur+post)
		if winFrom.IsZero() || from.Before(winFrom) {
			winFrom = from
		}
		if until.After(winUntil) {
			winUntil = until
		}
		// an oversized SEB only seats its Anny footprint here; the rest goes to the R-building
		demand, _ := preplanAnnyDemand(pe, start, own, blockDur, rBauSebThreshold)
		if pe.ExamKind == "EXaHM" {
			exahmDemand += demand
		}
		totalDemand += demand
		modules = append(modules, pe.Module)
		if !containsString(kinds, pe.ExamKind) {
			kinds = append(kinds, pe.ExamKind)
		}
	}

	// per-exam allowedRooms restrict the pool (only when EVERY exam of that kind restricts)
	allRooms := make([]preplancalc.RoomCapacity, 0, len(free))
	seenRoom := make(map[string]bool)
	for _, iv := range free {
		if n := preplancalc.NormRoomName(iv.room); !seenRoom[n] {
			seenRoom[n] = true
			allRooms = append(allRooms, preplancalc.RoomCapacity{Name: iv.room, Seats: iv.seats})
		}
	}
	allowedFor := func(kind string) map[string]bool {
		set := make(map[string]bool)
		for _, r := range preplancalc.RoomsForKind(exams, kind, allRooms) {
			set[preplancalc.NormRoomName(r.Name)] = true
		}
		return set
	}
	allowedExahm, allowedSeb := allowedFor("EXaHM"), allowedFor("SEB")

	// rooms already ours for the whole window cover part of the demand for free
	haveExahm, haveTotal := 0, 0
	ours := make(map[string]bool)
	for _, iv := range own {
		if !anny.Covers(iv.from, iv.until, winFrom, winUntil) {
			continue
		}
		ours[preplancalc.NormRoomName(iv.room)] = true
		if iv.exahm {
			haveExahm += iv.seats
			haveTotal += iv.seats
		} else if iv.seb {
			haveTotal += iv.sebSeats
		}
	}

	// candidates: free rooms covering the whole window, largest first
	cands := make([]bookedRoomInterval, 0)
	for _, iv := range free {
		if ours[preplancalc.NormRoomName(iv.room)] {
			continue
		}
		if anny.Covers(iv.from, iv.until, winFrom, winUntil) {
			cands = append(cands, iv)
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].seats != cands[j].seats {
			return cands[i].seats > cands[j].seats
		}
		return cands[i].room < cands[j].room
	})

	out := make([]*model.PreplanBookingSuggestion, 0)
	picked := make(map[string]bool)
	pick := func(iv bookedRoomInterval) {
		picked[preplancalc.NormRoomName(iv.room)] = true
		if iv.exahm {
			haveExahm += iv.seats
			haveTotal += iv.seats
		} else if iv.seb {
			haveTotal += iv.sebSeats
		}
		st := start
		out = append(out, &model.PreplanBookingSuggestion{
			Room: iv.room, From: winFrom, Until: winUntil, Seats: iv.seats,
			Starttimes: []*time.Time{&st}, Modules: modules, Kinds: kinds,
		})
	}
	// EXaHM first — it can only be seated in EXaHM rooms
	for _, iv := range cands {
		if haveExahm >= exahmDemand {
			break
		}
		if !iv.exahm || picked[preplancalc.NormRoomName(iv.room)] || !allowedExahm[preplancalc.NormRoomName(iv.room)] {
			continue
		}
		pick(iv)
	}
	// then top up the total demand with any Anny room
	for _, iv := range cands {
		if haveTotal >= totalDemand {
			break
		}
		if picked[preplancalc.NormRoomName(iv.room)] || !allowedSeb[preplancalc.NormRoomName(iv.room)] {
			continue
		}
		pick(iv)
	}
	return out
}

// mergeBookingSuggestions merges the per-slot suggestions of the same room into one booking
// per contiguous window (two adjacent slots in the same room = one Anny booking), keeping
// the covered slot start times, modules and kinds. Sorted by time, then room.
func mergeBookingSuggestions(in []*model.PreplanBookingSuggestion) []*model.PreplanBookingSuggestion {
	sorted := append([]*model.PreplanBookingSuggestion(nil), in...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Room != sorted[j].Room {
			return sorted[i].Room < sorted[j].Room
		}
		return sorted[i].From.Before(sorted[j].From)
	})
	merged := make([]*model.PreplanBookingSuggestion, 0, len(sorted))
	for _, s := range sorted {
		last := len(merged) - 1
		if last >= 0 && merged[last].Room == s.Room && !s.From.After(merged[last].Until) {
			if s.Until.After(merged[last].Until) {
				merged[last].Until = s.Until
			}
			merged[last].Starttimes = append(merged[last].Starttimes, s.Starttimes...)
			for _, m := range s.Modules {
				if !containsString(merged[last].Modules, m) {
					merged[last].Modules = append(merged[last].Modules, m)
				}
			}
			for _, k := range s.Kinds {
				if !containsString(merged[last].Kinds, k) {
					merged[last].Kinds = append(merged[last].Kinds, k)
				}
			}
			continue
		}
		merged = append(merged, s)
	}
	sort.Slice(merged, func(i, j int) bool {
		if !merged[i].From.Equal(merged[j].From) {
			return merged[i].From.Before(merged[j].From)
		}
		return merged[i].Room < merged[j].Room
	})
	return merged
}

// bookingProposalFindings summarizes the proposal in the same graded form as the pre-plan
// validation: what to book, what it buys, and what stays impossible.
func bookingProposalFindings(proposal *model.PreplanBookingProposal, preExams []*model.PreplanExam, keepAssigned bool) []*model.PreplanFinding {
	findings := make([]*model.PreplanFinding, 0, 3)
	mode := "ergänzend zu den vorhandenen Buchungen"
	if !keepAssigned {
		mode = "komplett neu geplant"
	}
	switch {
	case len(proposal.Suggestions) == 0 && proposal.UnplacedNow == 0:
		findings = append(findings, &model.PreplanFinding{
			Level:   model.ValidationLevelInfo,
			Message: "alle Prüfungen sind eingeplant und die gebuchten Räume reichen — keine weitere Buchung nötig",
		})
	case len(proposal.Suggestions) == 0:
		findings = append(findings, &model.PreplanFinding{
			Level:   model.ValidationLevelWarning,
			Message: "keine zusätzliche Buchung möglich — in den freien Anny-Fenstern ist kein passender Raum mehr verfügbar",
		})
	default:
		seats := 0
		for _, s := range proposal.Suggestions {
			seats += s.Seats
		}
		placed := len(preExams) - len(proposal.StillUnplacedIDs)
		msg := fmt.Sprintf("%s vorgeschlagen (%d Plätze, %s) — damit sind %d von %s eingeplant",
			pluralN(len(proposal.Suggestions), "Buchung", "Buchungen"), seats, mode,
			placed, pluralN(len(preExams), "Prüfung", "Prüfungen"))
		if n := len(proposal.NewlyPlaced); n > 0 {
			msg += fmt.Sprintf(", darunter %s ohne bisherigen Slot",
				pluralN(n, "Prüfung", "Prüfungen"))
		}
		findings = append(findings, &model.PreplanFinding{Level: model.ValidationLevelInfo, Message: msg})
	}
	if n := len(proposal.StillUnplacedIDs); n > 0 {
		big := 0 // must-place exams (EXaHM) among them — they make it an error
		byID := make(map[int]*model.PreplanExam, len(preExams))
		for _, pe := range preExams {
			byID[pe.ID] = pe
		}
		mods := make([]string, 0, n)
		for _, id := range proposal.StillUnplacedIDs {
			pe := byID[id]
			if pe == nil {
				continue
			}
			if pe.ExamKind != "SEB" {
				big++
			}
			mods = append(mods, pe.Module)
		}
		level, hint := model.ValidationLevelError, ""
		if big == 0 {
			// only SEB left over: those may be planned in the R-building instead
			level, hint = model.ValidationLevelWarning, " — SEB kann in den R-Bau ausweichen"
		}
		findings = append(findings, &model.PreplanFinding{
			Level: level,
			Message: fmt.Sprintf("%s bleibt ohne Slot, auch wenn alle freien Anny-Räume gebucht werden (%s)%s",
				pluralN(n, "Prüfung", "Prüfungen"), joinStrings(mods), hint),
		})
	}
	return findings
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
