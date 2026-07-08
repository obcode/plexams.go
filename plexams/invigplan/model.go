// Package invigplan contains the pure (DB- and ZPA-free) domain model and the
// hard/soft constraints used to plan invigilations (Aufsichten) automatically.
//
// The package is deliberately self-contained so that it can be unit-tested
// without MongoDB and reused by both the optimizer and the validation code: a
// Problem is a static snapshot of everything the planner needs, a Plan is the
// mutable assignment the optimizer searches over, and every constraint is its
// own small function/type registered in the Registry. Adding a new rule means
// adding a constraint and registering it – nothing else changes.
package invigplan

import "time"

// Unassigned marks a position that currently has no invigilator.
const Unassigned = -1

// Kind classifies a position for the even-distribution soft constraints.
type Kind int

const (
	KindRoom    Kind = iota // a normal exam room
	KindNTA                 // a room holding a single NTA student
	KindReserve             // the per-slot reserve
)

func (k Kind) String() string {
	switch k {
	case KindNTA:
		return "nta"
	case KindReserve:
		return "reserve"
	default:
		return "room"
	}
}

// Position is one invigilation that has to be filled: a room in a slot, or the
// reserve of a slot. Self-invigilations are modelled as positions too, but are
// always part of Problem.Fixed and therefore never moved by the optimizer.
type Position struct {
	Room      string // "" for the reserve
	IsReserve bool
	IsNTA     bool
	IsSelf    bool // self-invigilation (examiner supervising their own exam)

	// Minutes counts toward the invigilator's minute contingent. It is 0 for
	// self-invigilations, the exam duration for a normal room and (per the
	// existing model) 60 for a reserve.
	Minutes int

	// Block is the real time the position occupies, used for the time-gap and
	// the daily-span constraints. For a reserve or self-invigilation this is the
	// longest exam duration in the slot, otherwise the exam duration.
	Block int

	Start time.Time
}

// End returns the time the position is over (Start + Block).
func (p Position) End() time.Time {
	return p.Start.Add(time.Duration(p.Block) * time.Minute)
}

// SlotKey identifies the slot a position lives in by its absolute start time
// (Unix seconds). Absolute time is the source of truth – there are no day/slot
// ordinals.
func (p Position) SlotKey() int64 { return p.Start.Unix() }

// dateKey maps a time to a calendar-date ordinal (y*10000 + month*100 + day),
// so two times on the same calendar day share a key regardless of the time of
// day. It is used for all "day" keyed maps (ExcludedDays, OwnExamDays).
func dateKey(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}

// Kind classifies the position for distribution balancing.
func (p Position) Kind() Kind {
	switch {
	case p.IsReserve:
		return KindReserve
	case p.IsNTA:
		return KindNTA
	default:
		return KindRoom
	}
}

// Invigilator holds everything about a person needed to decide and score
// assignments. Slot keys are position start times (Unix seconds); day keys are
// calendar-date ordinals (see dateKey).
type Invigilator struct {
	ID            int
	TargetMinutes int // = InvigilatorTodos.TotalMinutes

	ExcludedDays  map[int]bool   // calendar-date ordinals (see dateKey)
	ExcludedSlots map[int64]bool // position start times (Unix seconds)

	// OwnExamSlots are slots in which this person has an own exam and therefore
	// must not take a (non-self) invigilation. For NTA exams running into the
	// following slot the builder also adds that following slot. Keyed by the
	// slot's start time (Unix seconds).
	OwnExamSlots map[int64]bool
	OwnExamDays  map[int]bool // calendar-date ordinals (see dateKey)

	// OwnExams are the time windows the person is present for their own exams –
	// including multi-room exams they do *not* invigilate themselves. They count
	// toward the daily presence span (daySpanSoft) so an early own exam plus a
	// late invigilation is recognised as a long day.
	OwnExams []TimeSpan

	// TimeWindows restrict, per calendar date, the times the person may
	// invigilate (see DayTimeWindow). Empty means unrestricted.
	TimeWindows []DayTimeWindow
}

// DayTimeWindow restricts the times an invigilator may invigilate on one
// calendar date: an assigned position must start no earlier than From (if set)
// and end no later than Until (if set). It is sub-slot granular and NTA-aware,
// since Position.End() already includes the room's (possibly NTA-extended)
// duration. Several windows may share a date – then the position only has to fit
// into one of them (see AllowsTime), e.g. 08:00–11:00 and 14:00–open to keep
// 11:00–14:00 free.
type DayTimeWindow struct {
	Date  time.Time // calendar date the window applies to
	From  time.Time // earliest allowed start; zero = no lower bound
	Until time.Time // latest allowed end; zero = no upper bound
}

// fits reports whether the position lies completely inside this window.
func (w DayTimeWindow) fits(pos Position) bool {
	if !w.From.IsZero() && pos.Start.Before(w.From) {
		return false
	}
	if !w.Until.IsZero() && pos.End().After(w.Until) {
		return false
	}
	return true
}

// AllowsTime reports whether the position fits the person's time windows. Only
// windows on the position's calendar date are considered; if there are any, the
// position must fit into at least one of them (windows on the same date are
// OR-combined). With no window for that date (or no windows at all) the position
// is allowed.
func (in *Invigilator) AllowsTime(pos Position) bool {
	hasWindowForDate := false
	for _, w := range in.TimeWindows {
		if !sameDate(w.Date, pos.Start) {
			continue
		}
		hasWindowForDate = true
		if w.fits(pos) {
			return true
		}
	}
	return !hasWindowForDate
}

// sameDate reports whether a and b fall on the same calendar day.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// TimeSpan is a presence interval on a given day.
type TimeSpan struct {
	Start time.Time
	End   time.Time
}

// Date returns the calendar-date ordinal of the span's start (see dateKey).
func (ts TimeSpan) Date() int { return dateKey(ts.Start) }

// Available reports whether the person may in principle invigilate at the
// position's time regarding their stated availability (excluded day/slot).
// Time-of-day restrictions are handled by timeWindowHard, the own-exam
// restriction by ownExamHard.
func (in *Invigilator) Available(pos Position) bool {
	return !in.ExcludedDays[dateKey(pos.Start)] && !in.ExcludedSlots[pos.Start.Unix()]
}

// Problem is the immutable snapshot the planner works on.
type Problem struct {
	Positions    []Position
	Invigilators []Invigilator

	// Fixed maps a position index to the invigilator it is locked to (pre-planned
	// invigilations and self-invigilations). Fixed positions are never moved.
	Fixed map[int]int

	TimelagMin   int     // minimum minutes between two invigilations (rooms.timelag)
	ToleranceMin int     // allowed deviation from TargetMinutes (default 60)
	MaxSpanHours float64 // max hours from first start to last end per day (default 8)
	Weights      Weights

	byID map[int]*Invigilator
}

// Prepare builds the internal indices. It must be called once before the
// Problem is used by a Plan or the constraints.
func (p *Problem) Prepare() {
	p.byID = make(map[int]*Invigilator, len(p.Invigilators))
	for i := range p.Invigilators {
		p.byID[p.Invigilators[i].ID] = &p.Invigilators[i]
	}
	if p.ToleranceMin == 0 {
		p.ToleranceMin = 60
	}
	if p.MaxSpanHours == 0 {
		p.MaxSpanHours = 8
	}
	if p.Weights == (Weights{}) {
		p.Weights = DefaultWeights()
	}
}

// Invigilator returns the invigilator with the given id, or nil.
func (p *Problem) Invigilator(id int) *Invigilator {
	return p.byID[id]
}
