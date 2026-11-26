package plexams

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/rs/zerolog/log"
)

// validPreplanExamKinds are the allowed examKind values.
var validPreplanExamKinds = map[string]bool{"EXaHM": true, "SEB": true}

// preplanDefaultDurationMinutes is the exam duration stored when none was entered: the
// ordinary 90-minute exam. Writing it down at creation time keeps the whole pre-planning
// (booking-window gate, capacity-over-time, findings) working on a real number instead of
// a derived guess — and the GUI shows the planner what is assumed, so they can correct it.
const preplanDefaultDurationMinutes = 90

// preplanInputDuration returns the duration to store for a pre-exam: the entered value, or
// the standard 90 minutes when nothing (or a nonsensical value) was entered.
func preplanInputDuration(entered *int) *int {
	if entered != nil && *entered > 0 {
		return entered
	}
	d := preplanDefaultDurationMinutes
	return &d
}

// PreplanExams returns all SEB/EXaHM pre-planning pseudo-exams of this semester.
func (p *Plexams) PreplanExams(ctx context.Context) ([]*model.PreplanExam, error) {
	return p.dbClient.PreplanExams(ctx)
}

// PreplanExam returns one pre-exam by id.
func (p *Plexams) PreplanExam(ctx context.Context, id int) (*model.PreplanExam, error) {
	return p.dbClient.PreplanExam(ctx, id)
}

// AddPreplanExam validates and creates a new pre-exam (the examer's name is snapshotted
// from the linked teacher).
func (p *Plexams) AddPreplanExam(ctx context.Context, input *model.PreplanExamInput) (*model.PreplanExam, error) {
	preplanExam, err := p.preplanExamFromInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return p.dbClient.InsertPreplanExam(ctx, preplanExam)
}

// UpdatePreplanExam validates and replaces an existing pre-exam, keeping its id, slot
// assignment and ancode link.
func (p *Plexams) UpdatePreplanExam(ctx context.Context, id int, input *model.PreplanExamInput) (*model.PreplanExam, error) {
	existing, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apierr.New(apierr.CodeNotFound, errPreplanExamGone)
	}

	preplanExam, err := p.preplanExamFromInput(ctx, input)
	if err != nil {
		return nil, err
	}
	// preserve fields not part of the input
	preplanExam.ID = existing.ID
	preplanExam.PlannedStarttime = existing.PlannedStarttime
	preplanExam.Ancode = existing.Ancode
	// Set by their own mutations (setPreplanExamConstraints, …NotSameSlot,
	// …CanShareSlot, …Fixed); ReplacePreplanExam writes the whole record, so
	// dropping them here wiped them on every edit of the form fields.
	preplanExam.IsFixed = existing.IsFixed
	preplanExam.NotSameSlot = existing.NotSameSlot
	preplanExam.CanShareSlot = existing.CanShareSlot
	preplanExam.Constraints = existing.Constraints

	if _, err := p.dbClient.ReplacePreplanExam(ctx, preplanExam); err != nil {
		return nil, err
	}
	return preplanExam, nil
}

// DeletePreplanExam removes a pre-exam by id.
func (p *Plexams) DeletePreplanExam(ctx context.Context, id int) (bool, error) {
	return p.dbClient.DeletePreplanExam(ctx, id)
}

// SetPreplanExamTime assigns (or, with nil, clears) the absolute start time of a
// pre-exam. Any time is accepted (the source of truth); a time that matches no booked
// slot simply leaves the exam effectively unplaced for capacity purposes.
func (p *Plexams) SetPreplanExamTime(ctx context.Context, id int, starttime *time.Time) (*model.PreplanExam, error) {
	preplanExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preplanExam == nil {
		return nil, apierr.New(apierr.CodeNotFound, errPreplanExamGone)
	}

	if starttime == nil {
		preplanExam.PlannedStarttime = nil
	} else {
		t := *starttime
		preplanExam.PlannedStarttime = &t
	}

	if _, err := p.dbClient.ReplacePreplanExam(ctx, preplanExam); err != nil {
		return nil, err
	}
	return preplanExam, nil
}

// ResetPreplanTimes clears the planned start time of every pre-exam that is NOT fixed,
// so the Vorplanung can be re-run from scratch. Fixed pre-exams and the pre-exams
// themselves are kept. Returns how many were reset.
func (p *Plexams) ResetPreplanTimes(ctx context.Context) (int, error) {
	preplanExams, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return 0, err
	}
	reset := 0
	for _, pe := range preplanExams {
		if pe.IsFixed || pe.PlannedStarttime == nil {
			continue
		}
		pe.PlannedStarttime = nil
		if err := p.dbClient.UpsertPreplanExam(ctx, pe); err != nil {
			return reset, err
		}
		reset++
	}
	return reset, nil
}

// SetPreplanExamFixed pins or unpins the pre-exam's current slot. A fixed pre-exam
// keeps its slot when the automatic assignment is regenerated. Fixing only makes
// sense for a slotted exam, so it is rejected when the exam has no slot.
func (p *Plexams) SetPreplanExamFixed(ctx context.Context, id int, fixed bool) (*model.PreplanExam, error) {
	preplanExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preplanExam == nil {
		return nil, apierr.New(apierr.CodeNotFound, errPreplanExamGone)
	}
	if fixed && preplanExam.PlannedStarttime == nil {
		return nil, apierr.New(apierr.CodeInvalidInput, "Diese Vorplanungs-Prüfung hat noch keinen Termin. Erst einen Termin zuweisen, dann fixieren.")
	}

	preplanExam.IsFixed = fixed
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preplanExam); err != nil {
		return nil, err
	}

	// keep the linked ZPA exam's LOCKED plan entry in sync with the fix, mirroring
	// ConnectPreplanExamToAncode / DisconnectPreplanExam: a fixed pre-exam pins its
	// linked ancode into that slot as a locked plan entry, un-fixing drops it again.
	// Without this the fix would stay dangling on the ZPA exam after un-fixing.
	if preplanExam.Ancode != nil {
		if fixed {
			if _, err := p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{
				Starttime: preplanExam.PlannedStarttime,
				Ancode:    *preplanExam.Ancode,
				Locked:    true,
			}); err != nil {
				return nil, wrapGerman(err, "Die verknüpfte ZPA-Prüfung (Ancode %d) ließ sich nicht fest auf %s setzen",
					*preplanExam.Ancode, slotLabelDE(*preplanExam.PlannedStarttime))
			}
		} else {
			if err := p.dbClient.RemovePlanEntry(ctx, *preplanExam.Ancode); err != nil {
				log.Error().Err(err).Int("ancode", *preplanExam.Ancode).
					Msg("cannot remove pre-planned slot on un-fix")
			}
		}
	}

	return preplanExam, nil
}

// SetPreplanExamNotSameSlot marks (conflict=true) or unmarks (false) otherID as a
// "must not run at the same time" partner of id (same students). The link is kept
// symmetric. Soft: the automatic assignment then spreads the two apart.
func (p *Plexams) SetPreplanExamNotSameSlot(ctx context.Context, id, otherID int, conflict bool) (*model.PreplanExam, error) {
	return p.setPreplanPairLink(ctx, id, otherID, conflict,
		func(pe *model.PreplanExam) []int { return pe.NotSameSlot },
		func(pe *model.PreplanExam, ids []int) { pe.NotSameSlot = ids })
}

// SetPreplanExamCanShareSlot marks (canShare=true) or unmarks (false) otherID as "may
// run at the same time / right after" id despite a shared study program. The link is
// kept symmetric and cancels the program-based spreading for that pair.
func (p *Plexams) SetPreplanExamCanShareSlot(ctx context.Context, id, otherID int, canShare bool) (*model.PreplanExam, error) {
	return p.setPreplanPairLink(ctx, id, otherID, canShare,
		func(pe *model.PreplanExam) []int { return pe.CanShareSlot },
		func(pe *model.PreplanExam, ids []int) { pe.CanShareSlot = ids })
}

// setPreplanPairLink adds (add=true) or removes a symmetric pre-exam link between id and
// otherID, using get/set to access the relevant id list on each pre-exam.
func (p *Plexams) setPreplanPairLink(ctx context.Context, id, otherID int, add bool,
	get func(*model.PreplanExam) []int, set func(*model.PreplanExam, []int)) (*model.PreplanExam, error) {
	if id == otherID {
		return nil, apierr.New(apierr.CodeInvalidInput, "Eine Vorplanungs-Prüfung kann nicht mit sich selbst verbunden werden.")
	}
	if add {
		if other, err := p.dbClient.PreplanExam(ctx, otherID); err != nil {
			return nil, err
		} else if other == nil {
			return nil, apierr.New(apierr.CodeNotFound, errPreplanExamGone)
		}
	}
	if err := p.updatePreplanPair(ctx, id, otherID, add, get, set); err != nil {
		return nil, err
	}
	if err := p.updatePreplanPair(ctx, otherID, id, add, get, set); err != nil {
		return nil, err
	}
	return p.dbClient.PreplanExam(ctx, id)
}

// updatePreplanPair adds or removes other from id's link list (one direction).
func (p *Plexams) updatePreplanPair(ctx context.Context, id, other int, add bool,
	get func(*model.PreplanExam) []int, set func(*model.PreplanExam, []int)) error {
	pe, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return err
	}
	if pe == nil {
		return apierr.New(apierr.CodeNotFound, errPreplanExamGone)
	}
	ids := make(map[int]bool, len(get(pe)))
	for _, x := range get(pe) {
		ids[x] = true
	}
	if add {
		ids[other] = true
	} else {
		delete(ids, other)
	}
	kept := make([]int, 0, len(ids))
	for x := range ids {
		kept = append(kept, x)
	}
	sort.Ints(kept)
	set(pe, kept)
	_, err = p.dbClient.ReplacePreplanExam(ctx, pe)
	return err
}

// preplanExamFromInput validates a PreplanExamInput and builds a PreplanExam, snapshotting the
// examer's name from the linked teacher.
func (p *Plexams) preplanExamFromInput(ctx context.Context, input *model.PreplanExamInput) (*model.PreplanExam, error) {
	if input == nil {
		return nil, apierr.New(apierr.CodeInvalidInput, "Es wurden keine Angaben zur Vorplanungs-Prüfung übermittelt.")
	}
	if !validPreplanExamKinds[input.ExamKind] {
		return nil, apierr.Newf(apierr.CodeInvalidInput, "Unbekannte Prüfungsart %q. Erlaubt sind EXaHM und SEB.", input.ExamKind)
	}
	if strings.TrimSpace(input.Module) == "" {
		return nil, apierr.New(apierr.CodeInvalidInput, "Bitte den Namen des Moduls angeben.")
	}
	if input.ExpectedStudents < 0 {
		return nil, apierr.New(apierr.CodeInvalidInput, "Die erwartete Zahl der Studierenden darf nicht negativ sein.")
	}

	examerName := ""
	teacher, err := p.GetTeacher(ctx, input.ExamerID)
	if err != nil {
		return nil, err
	}
	if teacher == nil {
		return nil, apierr.New(apierr.CodeNotFound, "Die gewählte Prüferin bzw. der gewählte Prüfer ist in der Liste der Lehrenden nicht (mehr) vorhanden. Lehrende unter „ZPA-Import“ neu laden und erneut auswählen.")
	}
	examerName = teacher.Fullname

	programs := make([]string, 0, len(input.Programs))
	for _, prog := range input.Programs {
		prog = strings.TrimSpace(prog)
		if prog != "" {
			programs = append(programs, prog)
		}
	}

	notes := ""
	if input.Notes != nil {
		notes = strings.TrimSpace(*input.Notes)
	}

	return &model.PreplanExam{
		ExamKind:         input.ExamKind,
		ExamerID:         input.ExamerID,
		ExamerName:       examerName,
		Module:           strings.TrimSpace(input.Module),
		Programs:         programs,
		ExpectedStudents: input.ExpectedStudents,
		Duration:         preplanInputDuration(input.Duration),
		Notes:            notes,
	}, nil
}
