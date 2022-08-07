package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetZPAExams(ctx context.Context, fromZpa *bool) ([]*model.ZPAExam, error) {
	if fromZpa != nil && *fromZpa {
		if err := p.SetZPA(); err != nil {
			return nil, err
		}

		exams := p.zpa.client.GetExams()

		err := p.dbClient.CacheZPAExams(exams, p.semester)
		if err != nil {
			return nil, err
		}
		return exams, nil
	} else {
		return p.dbClient.GetZPAExams(ctx)
	}
}

func (p *Plexams) GetZPAExamsGroupedByType(ctx context.Context) ([]*model.ZPAExamsForType, error) {
	exams, err := p.dbClient.GetZPAExams(ctx)
	if err != nil {
		return nil, err
	}

	examsByType := make(map[string][]*model.ZPAExam)

	for _, exam := range exams {
		v, ok := examsByType[exam.ExamType]
		if !ok {
			v = make([]*model.ZPAExam, 0)
		}
		examsByType[exam.ExamType] = append(v, exam)
	}

	examsGroupedByType := make([]*model.ZPAExamsForType, 0)
	for k, v := range examsByType {
		examsGroupedByType = append(examsGroupedByType, &model.ZPAExamsForType{
			Type:  k,
			Exams: v,
		})
	}

	return examsGroupedByType, nil
}

func (p *Plexams) GetZPAExamTypes(ctx context.Context) ([]*model.ZPAExamType, error) {
	examsByType, err := p.GetZPAExamsGroupedByType(ctx)
	if err != nil {
		return nil, err
	}

	examTypes := make([]*model.ZPAExamType, 0)
	for _, v := range examsByType {
		examTypes = append(examTypes, &model.ZPAExamType{Type: v.Type})
	}

	return examTypes, nil
}
