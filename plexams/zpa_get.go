package plexams

import (
	"context"
	"fmt"
	"sort"

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

	invigilators := make([]*model.ZPAInvigilator, 0, len(justInvigilators))
	for _, invig := range justInvigilators {
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

func (p *Plexams) GetFk07programs(ctx context.Context) ([]*model.FK07Program, error) {
	programs := make([]*model.FK07Program, 0, len(p.zpa.fk07programs))

	for _, p := range p.zpa.fk07programs {
		programs = append(programs, &model.FK07Program{Name: p})
	}

	return programs, nil
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

	ancodes := make([]*model.AnCode, 0)
	for _, exam := range exams {
		ancodes = append(ancodes, &model.AnCode{Ancode: exam.AnCode})
	}

	return ancodes, nil
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

	err = p.dbClient.SetZPAExamsToPlan(ctx, examsToPlan, examsNotToPlan)
	if err != nil {
		log.Error().Err(err).Msg("cannot set zpa exams to plan")
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

func (p *Plexams) ZpaExamsPlaningStatusUnknown(ctx context.Context) ([]*model.ZPAExam, error) {
	all, err := p.dbClient.GetZPAExams(ctx)
	if err != nil {
		return nil, err
	}
	toPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
	if err != nil {
		return nil, err
	}
	notToPlan, err := p.dbClient.GetZPAExamsNotToPlan(ctx)
	if err != nil {
		return nil, err
	}

	statusUnknown := make([]*model.ZPAExam, 0)

	for _, exam := range all {
		planned := false
		for _, examP := range toPlan {
			if exam.AnCode == examP.AnCode {
				planned = true
				break
			}
		}
		if planned {
			continue
		}
		for _, examP := range notToPlan {
			if exam.AnCode == examP.AnCode {
				planned = true
				break
			}
		}
		if !planned {
			statusUnknown = append(statusUnknown, exam)
		}
	}

	return statusUnknown, nil
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
	supervisorRequirements := p.zpa.client.GetSupervisorRequirements()
	if supervisorRequirements == nil {
		return nil, fmt.Errorf("cannot get supervisor requirements")
	}

	reqInterface := make([]interface{}, 0, len(supervisorRequirements))
	for _, req := range supervisorRequirements {
		reqInterface = append(reqInterface, req)
	}

	err := p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "invigilator_requirements"), reqInterface)
	if err != nil {
		log.Error().Err(err).Msg("cannot save invigilator requirements")
		return supervisorRequirements, err
	}
	return supervisorRequirements, nil
}
