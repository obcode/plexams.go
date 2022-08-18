package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetTeacher(ctx context.Context, id int) (*model.Teacher, error) {
	return p.dbClient.GetTeacher(ctx, id)
}

func (p *Plexams) GetTeachers(ctx context.Context, fromZpa *bool) ([]*model.Teacher, error) {
	if fromZpa != nil && *fromZpa {
		if err := p.SetZPA(); err != nil {
			return nil, err
		}

		teachers := p.zpa.client.GetTeachers()

		err := p.dbClient.CacheTeachers(teachers, p.semester)
		if err != nil {
			return nil, err
		}
		return teachers, nil
	} else {
		return p.dbClient.GetTeachers(ctx)
	}
}

func (p *Plexams) GetInvigilators(ctx context.Context) ([]*model.Teacher, error) {
	return p.dbClient.GetInvigilators(ctx)
}

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

func (p *Plexams) GetZpaAnCodes(ctx context.Context) ([]*model.AnCode, error) {
	f := false
	exams, err := p.GetZPAExams(ctx, &f)
	if err != nil {
		return nil, err
	}

	anCodes := make([]*model.AnCode, 0)
	for _, exam := range exams {
		anCodes = append(anCodes, &model.AnCode{AnCode: exam.AnCode})
	}

	return anCodes, nil
}

func (p *Plexams) GetZpaExamByAncode(ctx context.Context, anCode int) (*model.ZPAExam, error) {
	return p.dbClient.GetZpaExamByAncode(ctx, anCode)
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
