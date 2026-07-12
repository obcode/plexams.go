package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PlannedAncodes() error {
	exams, err := p.GetZpaExamsToPlan(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
		return err
	}

	for _, exam := range exams {
		fmt.Println(exam.AnCode)
	}
	return nil
}

func (p *Plexams) PlanEntries(ctx context.Context) ([]*model.PlanEntry, error) {
	return p.dbClient.PlanEntries(ctx)
}

func (p *Plexams) PlannedExam(ctx context.Context, ancode int) (*model.PlannedExam, error) {
	exam, err := p.AssembledExam(ctx, ancode)
	if err != nil {
		log.Debug().Err(err).Int("ancode", ancode).Msg("cannot get assembled exam")
		return nil, err
	}

	planEntry, err := p.dbClient.PlanEntry(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get planEntry for ancode")
		return nil, err
	}

	plannedRooms, err := p.dbClient.PlannedRoomsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get planned rooms for ancode")
		return nil, err
	}

	return &model.PlannedExam{
		Ancode:           exam.Ancode,
		ZpaExam:          exam.ZpaExam,
		PrimussExams:     exam.PrimussExams,
		Constraints:      exam.Constraints,
		Conflicts:        exam.Conflicts,
		StudentRegsCount: exam.StudentRegsCount,
		Ntas:             exam.Ntas,
		MaxDuration:      exam.MaxDuration,
		PlanEntry:        planEntry,
		PlannedRooms:     plannedRooms,
	}, err
}

func (p *Plexams) PlannedExams(ctx context.Context) ([]*model.PlannedExam, error) {
	exams, err := p.AssembledExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get assembled exams")
		return nil, err
	}

	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planEntries")
		return nil, err
	}

	planEntryMap := make(map[int]*model.PlanEntry)
	for _, planEntry := range planEntries {
		planEntryMap[planEntry.Ancode] = planEntry
	}

	plannedExams := make([]*model.PlannedExam, 0, len(exams))

	for _, exam := range exams {
		plannedRooms, err := p.dbClient.PlannedRoomsForAncode(ctx, exam.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot find planned rooms")
		}

		plannedExams = append(plannedExams,
			&model.PlannedExam{
				Ancode:           exam.Ancode,
				ZpaExam:          exam.ZpaExam,
				PrimussExams:     exam.PrimussExams,
				Constraints:      exam.Constraints,
				Conflicts:        exam.Conflicts,
				StudentRegsCount: exam.StudentRegsCount,
				Ntas:             exam.Ntas,
				MaxDuration:      exam.MaxDuration,
				PlanEntry:        planEntryMap[exam.Ancode],
				PlannedRooms:     plannedRooms,
			})
	}

	return plannedExams, nil
}

// ExamsNotOnSlotGrid returns the planned exams whose absolute start time is NOT one of the
// semester's standard slot start times (e.g. another faculty's exam placed at 11:00). A
// slot-by-slot grid (which queries examsAt for each configured slot start) never shows
// these, so the GUI needs them separately to surface them without anything going missing.
func (p *Plexams) ExamsNotOnSlotGrid(ctx context.Context) ([]*model.PlannedExam, error) {
	all, err := p.PlannedExams(ctx)
	if err != nil {
		return nil, err
	}
	onGrid := make(map[int64]bool, len(p.semesterConfig.Slots))
	for _, s := range p.semesterConfig.Slots {
		onGrid[s.Starttime.Unix()] = true
	}
	offGrid := make([]*model.PlannedExam, 0)
	for _, e := range all {
		if e.PlanEntry == nil || e.PlanEntry.Starttime == nil {
			continue // not placed at all → belongs to examsWithoutSlot, not here
		}
		if !onGrid[e.PlanEntry.Starttime.Unix()] {
			offGrid = append(offGrid, e)
		}
	}
	return offGrid, nil
}

func (p *Plexams) PlannedExamsByExamer(ctx context.Context, examerID int) ([]*model.PlannedExam, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return nil, err
	}

	plannedExamsForExamer := make([]*model.PlannedExam, 0)
	for _, plannedExam := range plannedExams {
		if plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			continue
		}
		if plannedExam.ZpaExam.MainExamerID == examerID {
			plannedExamsForExamer = append(plannedExamsForExamer, plannedExam)
		}
	}

	return plannedExamsForExamer, nil
}

func (p *Plexams) PlannedExamsForProgram(ctx context.Context, program string, onlyPlannedByMe bool) ([]*model.PlannedExam, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return nil, err
	}

	plannedExamsForProgram := make([]*model.PlannedExam, 0)
	for _, plannedExam := range plannedExams {
		if onlyPlannedByMe && plannedExam.Constraints != nil && plannedExam.Constraints.NotPlannedByMe {
			continue
		}
		for _, primussExam := range plannedExam.PrimussExams {
			if primussExam.Exam.Program == program && len(primussExam.StudentRegs) > 0 {
				plannedExamsForProgram = append(plannedExamsForProgram, plannedExam)
				break
			}
		}
	}

	return plannedExamsForProgram, nil
}
