package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// validPreplanExamKinds are the allowed examKind values.
var validPreplanExamKinds = map[string]bool{"EXaHM": true, "SEB": true}

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
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	preplanExam, err := p.preplanExamFromInput(ctx, input)
	if err != nil {
		return nil, err
	}
	// preserve fields not part of the input
	preplanExam.ID = existing.ID
	preplanExam.PlannedStarttime = existing.PlannedStarttime
	preplanExam.Ancode = existing.Ancode

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
		return nil, fmt.Errorf("pre-exam %d not found", id)
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

// SetPreplanExamFixed pins or unpins the pre-exam's current slot. A fixed pre-exam
// keeps its slot when the automatic assignment is regenerated. Fixing only makes
// sense for a slotted exam, so it is rejected when the exam has no slot.
func (p *Plexams) SetPreplanExamFixed(ctx context.Context, id int, fixed bool) (*model.PreplanExam, error) {
	preplanExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preplanExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}
	if fixed && preplanExam.PlannedStarttime == nil {
		return nil, fmt.Errorf("cannot fix pre-exam %d: it has no slot yet", id)
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
				return nil, fmt.Errorf("cannot pin ancode %d into slot %s: %w",
					*preplanExam.Ancode, preplanExam.PlannedStarttime.Format("02.01. 15:04"), err)
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
		return nil, fmt.Errorf("a pre-exam cannot be linked to itself")
	}
	if add {
		if other, err := p.dbClient.PreplanExam(ctx, otherID); err != nil {
			return nil, err
		} else if other == nil {
			return nil, fmt.Errorf("pre-exam %d not found", otherID)
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
		return fmt.Errorf("pre-exam %d not found", id)
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
		return nil, fmt.Errorf("no pre-exam provided")
	}
	if !validPreplanExamKinds[input.ExamKind] {
		return nil, fmt.Errorf("invalid examKind %q (expected EXaHM or SEB)", input.ExamKind)
	}
	if strings.TrimSpace(input.Module) == "" {
		return nil, fmt.Errorf("module is required")
	}
	if input.ExpectedStudents < 0 {
		return nil, fmt.Errorf("expectedStudents must not be negative")
	}

	examerName := ""
	teacher, err := p.GetTeacher(ctx, input.ExamerID)
	if err != nil {
		return nil, fmt.Errorf("cannot look up examer %d: %w", input.ExamerID, err)
	}
	if teacher == nil {
		return nil, fmt.Errorf("no teacher with id %d", input.ExamerID)
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
		Duration:         input.Duration,
		Notes:            notes,
	}, nil
}
