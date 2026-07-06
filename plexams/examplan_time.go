package plexams

import (
	"context"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/examplan"
	"github.com/rs/zerolog/log"
)

// Start-time-avoidance defaults, used when the generation config leaves a field unset (so
// an older stored config, or a fresh DB, still gets sensible behaviour).
const (
	defaultSlotTimeWeight         = 5.0     // penalty per registration, per hour outside the wanted window
	defaultSlotTimeWinterEarliest = "10:00" // winter: avoid a start time before this
	defaultSlotTimeSummerLatest   = "13:00" // summer: avoid a start time after this
	// tbauSlotTimePullFactor: in phase A (EXaHM/SEB into booked T-Bau slots) the booked
	// rooms are exempt from the avoidance — in summer entirely (they are climate-
	// controlled, so we go purely by the booking). In winter we still apply a gentle pull
	// towards later starts, using this fraction of the normal weight, so an 08:30 booking
	// is left empty when possible (and can then be dropped in favour of R-rooms).
	tbauSlotTimePullFactor = 0.4
)

// slotTimeMode is the resolved (semester-independent) behaviour of the start-time
// avoidance constraint for one generation run.
type slotTimeMode int

const (
	slotTimeOff    slotTimeMode = iota // no penalty
	slotTimeWinter                     // avoid early starts (before the morning limit)
	slotTimeSummer                     // avoid late starts (after the midday limit)
)

// isSummerSemester reports whether the current semester is a summer semester (SS). The
// semester string is like "2026 SS" (runtime) or "2026-SS" (create form) — both end in
// the SS/WS marker, so a suffix check is robust to either separator.
func (p *Plexams) isSummerSemester() bool {
	return strings.HasSuffix(strings.ToUpper(strings.TrimSpace(p.semester)), "SS")
}

// resolveSlotTimeMode maps the configured mode (AUTO/WINTER/SUMMER/OFF, default AUTO) to
// the concrete behaviour for this semester.
func (p *Plexams) resolveSlotTimeMode(mode model.SlotTimeConstraintMode) slotTimeMode {
	switch mode {
	case model.SlotTimeConstraintModeOff:
		return slotTimeOff
	case model.SlotTimeConstraintModeWinter:
		return slotTimeWinter
	case model.SlotTimeConstraintModeSummer:
		return slotTimeSummer
	default: // AUTO (or unset): follow the semester
		if p.isSummerSemester() {
			return slotTimeSummer
		}
		return slotTimeWinter
	}
}

// slotTimeSeverity computes the per-slot start-time avoidance severity (hours the slot's
// start lies outside the wanted window) and the weight to use, honouring the phase-A
// T-Bau exception:
//   - phase B (general schedule): full weight; winter avoids early, summer avoids late starts.
//   - phase A (booked T-Bau EXaHM/SEB): summer/off → no penalty (go by the booking); winter
//     → a gentle pull (reduced weight) towards later starts so an early booking is left empty
//     when possible.
//
// A nil severity or a zero weight disables the term.
func (p *Plexams) slotTimeSeverity(ctx context.Context, slots []examplan.Slot, roomPhase bool) ([]float64, float64) {
	cfg, err := p.GenerationConfig(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("cannot read generation config; start-time avoidance disabled for this run")
		return nil, 0
	}
	weight := cfg.SlotTimeWeight
	if weight <= 0 {
		weight = defaultSlotTimeWeight
	}
	return computeSlotTimeSeverity(
		p.resolveSlotTimeMode(cfg.SlotTimeMode), weight,
		parseDayMinutes(cfg.SlotTimeWinterEarliest, defaultSlotTimeWinterEarliest),
		parseDayMinutes(cfg.SlotTimeSummerLatest, defaultSlotTimeSummerLatest),
		slots, roomPhase)
}

// computeSlotTimeSeverity is the pure core of slotTimeSeverity (no config/DB): it turns a
// resolved mode, weight, and the morning/midday limits (minutes since midnight) into the
// per-slot severity and the effective weight, applying the phase-A T-Bau exception.
func computeSlotTimeSeverity(mode slotTimeMode, weight float64, earliestMin, latestMin int, slots []examplan.Slot, roomPhase bool) ([]float64, float64) {
	if mode == slotTimeOff || weight <= 0 {
		return nil, 0
	}
	if roomPhase {
		// T-Bau exception (booked EXaHM/SEB rooms): only the winter pull survives, softly;
		// in summer the booking decides (climate-controlled), so no penalty at all.
		if mode != slotTimeWinter {
			return nil, 0
		}
		weight *= tbauSlotTimePullFactor
	}
	sev := make([]float64, len(slots))
	for i, s := range slots {
		startMin := s.Start.Hour()*60 + s.Start.Minute()
		outside := 0
		switch mode {
		case slotTimeWinter:
			outside = earliestMin - startMin // start before the morning limit
		case slotTimeSummer:
			outside = startMin - latestMin // start after the midday limit
		}
		if outside > 0 {
			sev[i] = float64(outside) / 60.0
		}
	}
	return sev, weight
}

// parseDayMinutes parses an "HH:MM" time of day into minutes since midnight, falling back
// to parsing def on any error.
func parseDayMinutes(s, def string) int {
	if t, err := time.Parse("15:04", strings.TrimSpace(s)); err == nil {
		return t.Hour()*60 + t.Minute()
	}
	t, _ := time.Parse("15:04", def)
	return t.Hour()*60 + t.Minute()
}
