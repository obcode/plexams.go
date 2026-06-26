package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

// AdditionalExams returns all additional (publish-only) exams.
func (p *Plexams) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	return p.dbClient.AdditionalExams(ctx)
}

// UpsertAdditionalExam creates or updates one additional exam (key: ancode).
func (p *Plexams) UpsertAdditionalExam(ctx context.Context, exam *model.AdditionalExam) (*model.AdditionalExam, error) {
	if exam.Ancode == 0 {
		return nil, fmt.Errorf("ancode is required")
	}
	if exam.Rooms == nil {
		exam.Rooms = []*model.AdditionalExamRoom{}
	}
	if err := p.dbClient.UpsertAdditionalExam(ctx, exam); err != nil {
		return nil, err
	}
	return exam, nil
}

// DeleteAdditionalExam removes one additional exam by ancode.
func (p *Plexams) DeleteAdditionalExam(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.DeleteAdditionalExam(ctx, ancode)
}

// additionalExamPlans returns the additional exams as ZPA upload entries.
func (p *Plexams) additionalExamPlans(ctx context.Context) ([]*model.ZPAExamPlan, error) {
	additionalExams, err := p.dbClient.AdditionalExams(ctx)
	if err != nil {
		return nil, err
	}
	plans := make([]*model.ZPAExamPlan, 0, len(additionalExams))
	for _, exam := range additionalExams {
		rooms := make([]*model.ZPAExamPlanRoom, 0, len(exam.Rooms))
		for _, room := range exam.Rooms {
			rooms = append(rooms, &model.ZPAExamPlanRoom{
				RoomName:      room.RoomName,
				InvigilatorID: room.InvigilatorID,
				Duration:      room.Duration,
				IsReserve:     room.IsReserve,
				StudentCount:  room.StudentCount,
				IsHandicap:    room.IsHandicap,
			})
		}
		plans = append(plans, &model.ZPAExamPlan{
			Semester:             p.semester,
			AnCode:               exam.Ancode,
			Date:                 exam.Date,
			Time:                 exam.Time,
			ReserveInvigilatorID: 0,
			Rooms:                rooms,
		})
	}
	return plans, nil
}
