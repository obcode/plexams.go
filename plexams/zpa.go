package plexams

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
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

		err := p.dbClient.CacheZPAExams(exams)
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
		v, ok := examsByType[exam.ExamTypeFull]
		if !ok {
			v = make([]*model.ZPAExam, 0)
		}
		examsByType[exam.ExamTypeFull] = append(v, exam)
	}

	keys := make([]string, 0, len(examsByType))
	for k := range examsByType {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	examsGroupedByType := make([]*model.ZPAExamsForType, 0)
	for _, k := range keys {
		examsGroupedByType = append(examsGroupedByType, &model.ZPAExamsForType{
			Type:  k,
			Exams: examsByType[k],
		})
	}

	return examsGroupedByType, nil
}

func (p *Plexams) ZpaExamsToPlan(ctx context.Context, input []int) ([]*model.ZPAExam, error) {
	f := false
	allExams, err := p.GetZPAExams(ctx, &f)
	if err != nil {
		log.Error().Err(err).Msg("cannot get all zpa exams")
		return nil, err
	}

	examsToPlan := make([]*model.ZPAExam, 0)
	examsNotToPlan := make([]*model.ZPAExam, 0)

	for _, exam := range allExams {
		if contained(exam.AnCode, input) {
			examsToPlan = append(examsToPlan, exam)
		} else {
			examsNotToPlan = append(examsNotToPlan, exam)
		}
	}

	err = p.dbClient.SetZPAExamsToPlan(ctx, examsToPlan)
	if err != nil {
		log.Error().Err(err).Msg("cannot set zpa exams to plan")
		return nil, err
	}

	err = p.dbClient.SetZPAExamsNotToPlan(ctx, examsNotToPlan)
	if err != nil {
		log.Error().Err(err).Msg("cannot set zpa exams not to plan")
		return nil, err
	}

	return examsToPlan, nil
}

func contained(ancode int, ancodes []int) bool {
	for _, ac := range ancodes {
		if ancode == ac {
			return true
		}
	}
	return false
}

func (p *Plexams) GetZpaExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return p.dbClient.GetZPAExamsToPlan(ctx)
}

func (p *Plexams) GetZpaExamsNotToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return p.dbClient.GetZPAExamsNotToPlan(ctx)
}
