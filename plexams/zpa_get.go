package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GetTeacher(ctx context.Context, id int) (*model.Teacher, error) {
	return p.dbClient.GetTeacher(ctx, id)
}

func (p *Plexams) GetTeacherIdByRegex(ctx context.Context, name string) (int, error) {
	return p.dbClient.GetTeacherIdByRegex(ctx, name)
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

func (p *Plexams) GetInvigilators(ctx context.Context) ([]*model.ZPAInvigilator, error) {
	justInvigilators, err := p.dbClient.GetInvigilators(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get invigilators")
		return nil, err
	}

	supervisorReqs, err := p.GetSupervisorRequirements(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get supervisor requirements")
		return nil, err
	}

	isNotInvigilator, _, err := p.notInvigilating(ctx)
	if err != nil {
		return nil, err
	}

	invigilators := make([]*model.ZPAInvigilator, 0, len(justInvigilators))
	for _, invig := range justInvigilators {
		if isNotInvigilator(invig.ID) {
			log.Debug().Str("name", invig.Shortname).Msg("is not invigilator")
			continue
		}

		invigilator := model.ZPAInvigilator{
			Teacher: invig,
		}
		for _, req := range supervisorReqs {
			if req.InvigilatorID == invig.ID {
				invigilator.HasSubmittedRequirements = true
				break
			}
		}
		invigilators = append(invigilators, &invigilator)
	}

	return invigilators, nil
}

func (p *Plexams) getInvigilators(ctx context.Context) ([]*model.Teacher, error) {
	return p.dbClient.GetInvigilators(ctx)
}

func (p *Plexams) GetStudents(ctx context.Context, mtknr string) ([]*model.ZPAStudent, error) {
	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	return p.zpa.client.GetStudents(mtknr)
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

func (p *Plexams) GetZpaAnCodesToPlan(ctx context.Context) ([]*model.AnCode, error) {
	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		return nil, err
	}

	ancodes := make([]*model.AnCode, 0)
	for _, exam := range exams {
		ancodes = append(ancodes, &model.AnCode{Ancode: exam.AnCode})
	}

	return ancodes, nil
}

func (p *Plexams) GetZpaExamByAncode(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	return p.dbClient.GetZpaExamByAncode(ctx, ancode)
}

func (p *Plexams) GetZpaExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	return p.dbClient.GetZPAExamsToPlan(ctx)
}

func (p *Plexams) AddZpaExamToPlan(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.AddZpaExamToPlan(ctx, ancode)
}

func (p *Plexams) RmZpaExamFromPlan(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.RmZpaExamFromPlan(ctx, ancode)
}

func (p *Plexams) GetSupervisorRequirements(ctx context.Context) ([]*zpa.SupervisorRequirements, error) {
	if err := p.SetZPA(); err != nil {
		return nil, err
	}
	supervisorRequirements, err := p.zpa.client.GetSupervisorRequirements()
	if err != nil {
		return nil, fmt.Errorf("cannot get supervisor requirements: %w", err)
	}

	reqInterface := make([]interface{}, 0, len(supervisorRequirements))
	for _, req := range supervisorRequirements {
		reqInterface = append(reqInterface, req)
	}

	err = p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "invigilator_requirements"), reqInterface)
	if err != nil {
		log.Error().Err(err).Msg("cannot save invigilator requirements")
		return supervisorRequirements, err
	}
	return supervisorRequirements, nil
}
