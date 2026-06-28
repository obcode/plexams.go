package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
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
	preplanExam.PlannedDayNumber = existing.PlannedDayNumber
	preplanExam.PlannedSlotNumber = existing.PlannedSlotNumber
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

// SetPreplanExamSlot assigns (or, with nil day/slot, clears) the slot of a pre-exam.
// A given slot must be a real slot of the semester.
func (p *Plexams) SetPreplanExamSlot(ctx context.Context, id int, dayNumber, slotNumber *int) (*model.PreplanExam, error) {
	preplanExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preplanExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	switch {
	case dayNumber == nil && slotNumber == nil:
		preplanExam.PlannedDayNumber = nil
		preplanExam.PlannedSlotNumber = nil
	case dayNumber != nil && slotNumber != nil:
		if _, err := p.GetStarttime(*dayNumber, *slotNumber); err != nil {
			return nil, fmt.Errorf("invalid slot (%d/%d): %w", *dayNumber, *slotNumber, err)
		}
		preplanExam.PlannedDayNumber = dayNumber
		preplanExam.PlannedSlotNumber = slotNumber
	default:
		return nil, fmt.Errorf("dayNumber and slotNumber must both be set or both be empty")
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
	if fixed && (preplanExam.PlannedDayNumber == nil || preplanExam.PlannedSlotNumber == nil) {
		return nil, fmt.Errorf("cannot fix pre-exam %d: it has no slot yet", id)
	}

	preplanExam.IsFixed = fixed
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preplanExam); err != nil {
		return nil, err
	}
	return preplanExam, nil
}

// SetPreplanExamNotSameSlot marks (conflict=true) or unmarks (false) otherID as a
// "must not run at the same time" partner of id (same students). The link is kept
// symmetric. Soft: the automatic assignment then spreads the two apart.
func (p *Plexams) SetPreplanExamNotSameSlot(ctx context.Context, id, otherID int, conflict bool) (*model.PreplanExam, error) {
	if id == otherID {
		return nil, fmt.Errorf("a pre-exam cannot conflict with itself")
	}
	if conflict {
		if other, err := p.dbClient.PreplanExam(ctx, otherID); err != nil {
			return nil, err
		} else if other == nil {
			return nil, fmt.Errorf("pre-exam %d not found", otherID)
		}
	}
	if err := p.updateNotSameSlot(ctx, id, otherID, conflict); err != nil {
		return nil, err
	}
	if err := p.updateNotSameSlot(ctx, otherID, id, conflict); err != nil {
		return nil, err
	}
	return p.dbClient.PreplanExam(ctx, id)
}

// updateNotSameSlot adds or removes other from id's NotSameSlot list (one direction).
func (p *Plexams) updateNotSameSlot(ctx context.Context, id, other int, add bool) error {
	pe, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return err
	}
	if pe == nil {
		return fmt.Errorf("pre-exam %d not found", id)
	}
	set := make(map[int]bool, len(pe.NotSameSlot))
	for _, x := range pe.NotSameSlot {
		set[x] = true
	}
	if add {
		set[other] = true
	} else {
		delete(set, other)
	}
	kept := make([]int, 0, len(set))
	for x := range set {
		kept = append(kept, x)
	}
	sort.Ints(kept)
	pe.NotSameSlot = kept
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
