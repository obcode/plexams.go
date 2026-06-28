package plexams

import (
	"context"
	"fmt"
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
