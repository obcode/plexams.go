package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// skipSlotTimeWindowOff is the skip reason when the start-time window is disabled in the
// generation config (slotTimeMode = OFF) — there is nothing to validate then.
const skipSlotTimeWindowOff = "Tageszeit-Fenster ist deaktiviert (slotTimeMode = OFF)"

// ValidateSemesterTimes checks the generated Terminplan against the semester-dependent
// start-time window (winter: not before slotTimeWinterEarliest; summer: not after
// slotTimeSummerLatest). Our own, non-exempt exams outside the window are graded by the
// configured enforcement (HARD → error: this should never happen; SOFT → warning: a
// deliberate deviation). EXaHM/SEB exams run in booked, climate-controlled T-Bau rooms and
// are exempt from the window — a placement outside it is only reported as INFO. External /
// not-planned-by-me exams are skipped (not ours to control).
func (p *Plexams) ValidateSemesterTimes(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "semester-times", "prüfe Semester-Zeiten (Tageszeit-Fenster)")

	if ok, err := p.planGenerated(ctx); err != nil {
		return nil, err
	} else if !ok {
		return v.skip(skipNoPlan), nil
	}

	cfg, err := p.GenerationConfig(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generation config")
		return nil, err
	}
	mode := p.resolveSlotTimeMode(cfg.SlotTimeMode)
	if mode == slotTimeOff {
		return v.skip(skipSlotTimeWindowOff), nil
	}
	earliestMin := parseDayMinutes(cfg.SlotTimeWinterEarliest, defaultSlotTimeWinterEarliest)
	latestMin := parseDayMinutes(cfg.SlotTimeSummerLatest, defaultSlotTimeSummerLatest)
	hard := cfg.SlotTimeEnforcement != model.SlotTimeConstraintEnforcementSoft

	// window rule text for the messages (self-explanatory per season).
	var windowRule, season string
	switch mode {
	case slotTimeWinter:
		windowRule = fmt.Sprintf("Beginn erst ab %02d:%02d erlaubt (Winter)", earliestMin/60, earliestMin%60)
		season = "Winter"
	case slotTimeSummer:
		windowRule = fmt.Sprintf("Beginn nur bis %02d:%02d erlaubt (Sommer)", latestMin/60, latestMin%60)
		season = "Sommer"
	}

	planEntries, err := p.PlanEntries(ctx)
	if err != nil {
		return nil, err
	}
	constraints, err := p.ConstraintsMap(ctx)
	if err != nil {
		return nil, err
	}

	v.step("prüfe Startzeiten gegen das %s-Fenster", season)
	for _, pe := range planEntries {
		if pe.Starttime == nil || pe.External {
			continue // not placed, or an external-time exam we do not control
		}
		c := constraints[pe.Ancode]
		if c != nil && c.NotPlannedByMe {
			continue // planned by another faculty — not our window
		}

		startMin := pe.Starttime.Hour()*60 + pe.Starttime.Minute()
		outside := false
		switch mode {
		case slotTimeWinter:
			outside = startMin < earliestMin
		case slotTimeSummer:
			outside = startMin > latestMin
		}
		if !outside {
			continue
		}

		r := ref{Ancode: ptr(pe.Ancode), Starttime: pe.Starttime}
		exempt := c != nil && c.RoomConstraints != nil && (c.RoomConstraints.Exahm || c.RoomConstraints.Seb)
		switch {
		case exempt:
			v.infof(r, "EXaHM/SEB-Prüfung %d beginnt %s – außerhalb des Tageszeit-Fensters (%s), aber klimatisierter T-Bau-Raum → zulässig",
				pe.Ancode, fmtStart(pe.Starttime), windowRule)
		case hard:
			v.errorf(r, "Prüfung %d beginnt %s – außerhalb des Tageszeit-Fensters (%s)",
				pe.Ancode, fmtStart(pe.Starttime), windowRule)
		default:
			v.warnf(r, "Prüfung %d beginnt %s – außerhalb des Tageszeit-Fensters (%s), SOFT-Modus: bewusste Abweichung",
				pe.Ancode, fmtStart(pe.Starttime), windowRule)
		}
	}

	return v.finish(), nil
}
