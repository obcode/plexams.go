package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/examplan"
	"github.com/rs/zerolog/log"
)

// Start-time-window defaults, used when the generation config leaves a field unset (so an
// older stored config, or a fresh DB, still gets sensible behaviour).
const (
	// defaultSlotTimeWeight is the SOFT-mode window penalty (per registration, per hour a
	// non-exempt exam starts outside its window). High so that even in SOFT mode the solver
	// exhausts every in-window option before deliberately deviating; below Unplaced (1e6),
	// so a placement-with-deviation still beats leaving the exam unplaced.
	defaultSlotTimeWeight = 20000.0
	// defaultSlotTimeGradientWeight is the mild "earlier is better" summer pull below the
	// cutoff (per registration, per hour later than the day's first slot). Small — it only
	// breaks ties inside the allowed window and pulls the large exams to the front.
	defaultSlotTimeGradientWeight = 2.0
	defaultSlotTimeWinterEarliest = "10:00" // winter: exams must not start before this
	defaultSlotTimeSummerLatest   = "14:00" // summer: exams must not start after this
)

// slotTimeMode is the resolved (semester-independent) behaviour of the start-time
// constraint for one generation run.
type slotTimeMode int

const (
	slotTimeOff    slotTimeMode = iota // no window
	slotTimeWinter                     // exams must not start before the morning limit
	slotTimeSummer                     // exams must not start after the afternoon limit (+ mild early pull)
)

// slotTimeSpec is the resolved start-time constraint for one generation run: the per-slot
// soft severity and its scalar weight (for the solver's TimeOfDay term), plus, when the
// window is enforced HARD, the per-slot "outside the window" flags used to restrict each
// non-exempt exam's slot domain. mode/earliest/latest are carried for the solver's
// violation messages. A zero weight (and nil forbidden) disables the whole term.
type slotTimeSpec struct {
	severity    []float64               // per-slot soft severity (hours of badness)
	weight      float64                 // scalar TimeOfDay weight; 0 = term off
	hard        bool                    // true: apply the window as a domain restriction
	forbidden   []bool                  // per-slot: start lies outside the window (only when hard)
	mode        examplan.TimeWindowMode // off/winter/summer, for reporting
	earliestMin int                     // winter morning limit (clock minutes)
	latestMin   int                     // summer afternoon limit (clock minutes)
}

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

// slotTimeSpec resolves the start-time constraint for this run from the generation config
// and the run's slots. EXaHM/SEB exemption is handled per-unit in the solver (and the
// caller skips exempt units when applying the HARD domain restriction), so it is not baked
// into the per-slot arrays here.
func (p *Plexams) slotTimeSpec(ctx context.Context, slots []examplan.Slot) slotTimeSpec {
	cfg, err := p.GenerationConfig(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("cannot read generation config; start-time window disabled for this run")
		return slotTimeSpec{}
	}
	windowWeight := cfg.SlotTimeWeight
	if windowWeight <= 0 {
		windowWeight = defaultSlotTimeWeight
	}
	gradientWeight := cfg.SlotTimeGradientWeight
	if gradientWeight <= 0 {
		gradientWeight = defaultSlotTimeGradientWeight
	}
	hard := cfg.SlotTimeEnforcement != model.SlotTimeConstraintEnforcementSoft // default HARD
	return computeSlotTimeSpec(
		p.resolveSlotTimeMode(cfg.SlotTimeMode), hard, windowWeight, gradientWeight,
		parseDayMinutes(cfg.SlotTimeWinterEarliest, defaultSlotTimeWinterEarliest),
		parseDayMinutes(cfg.SlotTimeSummerLatest, defaultSlotTimeSummerLatest),
		slots)
}

// computeSlotTimeSpec is the pure core of slotTimeSpec (no config/DB): it turns the resolved
// mode, enforcement and window limits into the per-slot severity, the scalar weight and the
// per-slot "outside the window" flags.
//
//   - winter: the window is [earliestMin, ∞) — a start before earliestMin is outside it.
//   - summer: the window is (-∞, latestMin] — a start after latestMin is outside it; within
//     the window a mild gradient (hours later than the day's first slot) pulls the large
//     exams to the front.
//
// HARD: the window is a domain restriction (forbidden[s] = outside), so the solver only sees
// the mild gradient (winter → none). SOFT: no domain restriction (forbidden nil); the window
// is a strong penalty folded into the severity together with the mild gradient, both driven
// by the single scalar weight.
func computeSlotTimeSpec(mode slotTimeMode, hard bool, windowWeight, gradientWeight float64, earliestMin, latestMin int, slots []examplan.Slot) slotTimeSpec {
	if mode == slotTimeOff || len(slots) == 0 {
		return slotTimeSpec{}
	}
	// earliest start of day (clock minutes) — the reference for the summer gradient. All
	// exam days share the same slot grid, so a single global minimum is each day's minimum.
	earliestStart := 24 * 60
	for _, s := range slots {
		if m := s.Start.Hour()*60 + s.Start.Minute(); m < earliestStart {
			earliestStart = m
		}
	}
	windowHours := make([]float64, len(slots))
	gradientHours := make([]float64, len(slots))
	for i, s := range slots {
		startMin := s.Start.Hour()*60 + s.Start.Minute()
		switch mode {
		case slotTimeWinter:
			if d := earliestMin - startMin; d > 0 {
				windowHours[i] = float64(d) / 60.0
			}
		case slotTimeSummer:
			if d := startMin - latestMin; d > 0 {
				windowHours[i] = float64(d) / 60.0
			}
			if d := startMin - earliestStart; d > 0 {
				gradientHours[i] = float64(d) / 60.0
			}
		}
	}

	spec := slotTimeSpec{hard: hard, earliestMin: earliestMin, latestMin: latestMin}
	switch mode {
	case slotTimeWinter:
		spec.mode = examplan.TimeWindowWinter
	case slotTimeSummer:
		spec.mode = examplan.TimeWindowSummer
	}

	if hard {
		// window handled by the domain restriction; the solver only sees the mild gradient.
		spec.forbidden = make([]bool, len(slots))
		hasSeverity := false
		for i := range slots {
			spec.forbidden[i] = windowHours[i] > 0
			if gradientHours[i] > 0 {
				hasSeverity = true
			}
		}
		if hasSeverity && gradientWeight > 0 {
			spec.severity = gradientHours
			spec.weight = gradientWeight
		}
		return spec
	}

	// SOFT: no domain restriction; fold the (strong) window penalty and the (mild) gradient
	// into one severity array driven by the single window weight.
	sev := make([]float64, len(slots))
	ratio := 0.0
	if windowWeight > 0 {
		ratio = gradientWeight / windowWeight
	}
	any := false
	for i := range slots {
		sev[i] = windowHours[i] + ratio*gradientHours[i]
		if sev[i] > 0 {
			any = true
		}
	}
	if any && windowWeight > 0 {
		spec.severity = sev
		spec.weight = windowWeight
	}
	return spec
}

// timeWindowBoundText describes the active window bound for an unplaceable-reason message,
// e.g. "nicht vor 10:00" (winter) or "nicht nach 14:00" (summer).
func timeWindowBoundText(spec slotTimeSpec) string {
	switch spec.mode {
	case examplan.TimeWindowWinter:
		return fmt.Sprintf("nicht vor %02d:%02d", spec.earliestMin/60, spec.earliestMin%60)
	case examplan.TimeWindowSummer:
		return fmt.Sprintf("nicht nach %02d:%02d", spec.latestMin/60, spec.latestMin%60)
	}
	return "Zeitfenster"
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
