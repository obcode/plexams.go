package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetConnectedExam(ctx context.Context, anCode int) (*model.ConnectedExam, error) {
	zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, anCode)
	if err != nil {
		return nil, err
	}

	primussExams, _ := p.GetPrimussExamsForAncode(ctx, anCode)

	return &model.ConnectedExam{
		ZpaExam:      zpaExam,
		PrimussExams: primussExams,
	}, nil
}

func (p *Plexams) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	anCodes, err := p.GetZpaAnCodes(ctx)
	if err != nil {
		return nil, err
	}

	exams := make([]*model.ConnectedExam, 0)
	for _, anCode := range anCodes {
		exam, err := p.GetConnectedExam(ctx, anCode.AnCode)
		if err != nil {
			return exams, err
		}
		exams = append(exams, exam)
	}

	return exams, nil
}
