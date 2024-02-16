package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PlanEntries(ctx context.Context) ([]*model.PlanEntry, error) {
	return p.dbClient.PlanEntries(ctx)
}

func (p *Plexams) PlannedExam(ctx context.Context, ancode int) (*model.PlannedExam, error) {
	exam, err := p.GeneratedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get generated exam")
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
	exams, err := p.GeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated exams")
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
			break
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
			if primussExam.Exam.Program == program {
				plannedExamsForProgram = append(plannedExamsForProgram, plannedExam)
				break
			}
		}
	}

	return plannedExamsForProgram, nil
}
