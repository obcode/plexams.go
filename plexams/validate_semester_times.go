package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// skipSlotTimeWindowOff is the skip reason when the start-time window is disabled in the
// generation config (slotTimeMode = OFF) — there is nothing to validate then.
const skipSlotTimeWindowOff = "Das Tageszeit-Fenster ist ausgeschaltet (unter „Terminplan generieren“ steht es auf „Aus“)."

// ValidateSemesterTimes checks the generated Terminplan against the semester-dependent
// start-time window (winter: not before slotTimeWinterEarliest; summer: not after
// slotTimeSummerLatest). Our own, non-exempt exams outside the window are graded by the
// configured enforcement (HARD → error: this should never happen; SOFT → warning: a
// deliberate deviation). EXaHM/SEB exams run in booked, climate-controlled T-Bau rooms and
// are exempt from the window — a placement outside it is only reported as INFO. External /
// not-planned-by-me exams are skipped (not ours to control).
func (p *Plexams) ValidateSemesterTimes(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "semester-times", "prüfe die Startzeiten gegen das Tageszeit-Fenster")

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
		windowRule = fmt.Sprintf("im Winter Beginn frühestens %02d:%02d", earliestMin/60, earliestMin%60)
		season = "Winter"
	case slotTimeSummer:
		windowRule = fmt.Sprintf("im Sommer Beginn spätestens %02d:%02d", latestMin/60, latestMin%60)
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

	v.step("prüfe die Startzeiten gegen das %s-Fenster", season)
	examNames := p.newExamNamer(ctx, nil)
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
		name := examNames.name(pe.Ancode)
		exempt := c != nil && c.RoomConstraints != nil && (c.RoomConstraints.Exahm || c.RoomConstraints.Seb)
		switch {
		case exempt:
			v.infof(r, "%s beginnt am %s außerhalb des Tageszeit-Fensters (%s). Als Rechnerprüfung im klimatisierten T-Bau ist das erlaubt.",
				name, slotLabelDE(*pe.Starttime), windowRule)
		case hard:
			v.errorf(r.hint("Im Prüfungsplan die Prüfung auf eine Startzeit innerhalb des Fensters legen. Das Fenster stellst du unter „Terminplan generieren“ ein."),
				"%s beginnt am %s außerhalb des Tageszeit-Fensters (%s). Das Fenster ist als „hart“ eingestellt, dürfte also nicht verletzt sein.",
				name, slotLabelDE(*pe.Starttime), windowRule)
		default:
			v.warnf(r.hint("Prüfen, ob die Abweichung gewollt ist; sonst im Prüfungsplan die Prüfung auf eine Startzeit innerhalb des Fensters legen."),
				"%s beginnt am %s außerhalb des Tageszeit-Fensters (%s). Das Fenster ist als „weich“ eingestellt, Abweichungen sind also möglich.",
				name, slotLabelDE(*pe.Starttime), windowRule)
		}
	}

	return v.finish(), nil
}
