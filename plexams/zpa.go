package plexams

import (
	"context"
	"encoding/json"
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

func (p *Plexams) AddZpaExamToPlan(ctx context.Context, anCode int) (bool, error) {
	return p.dbClient.AddZpaExamToPlan(ctx, anCode)
}

func (p *Plexams) RmZpaExamFromPlan(ctx context.Context, anCode int) (bool, error) {
	return p.dbClient.RmZpaExamFromPlan(ctx, anCode)
}

func (p *Plexams) PostStudentRegsToZPA(ctx context.Context) (int, []*model.RegWithError, error) {
	if err := p.SetZPA(); err != nil {
		return 0, nil, err
	}

	zpaStudentRegs := make([]*model.ZPAStudentReg, 0)

	for _, program := range p.zpa.studentRegsForProgram {
		studentRegs, err := p.dbClient.StudentRegsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("error while getting student regs")
			return 0, nil, err
		}
		for _, studentReg := range studentRegs {
			zpaStudentRegs = append(zpaStudentRegs, p.zpa.client.StudentReg2ZPAStudentReg(studentReg))
		}
	}

	_, body, err := p.zpa.client.PostStudentRegsToZPA(zpaStudentRegs)
	if err != nil {
		log.Error().Err(err).Msg("error while posting student regs to zpa")
		return 0, nil, err
	}

	zpaStudentRegErrors := make([]*model.ZPAStudentRegError, 0)
	err = json.Unmarshal(body, &zpaStudentRegErrors)
	if err != nil {
		log.Error().Err(err).Msg("error while unmarshalling errors from ZPA")
		return 0, nil, err
	}

	regsWithErrors := make([]*model.RegWithError, 0)

	for i, e := range zpaStudentRegErrors {
		if !noError(e) {
			regsWithErrors = append(regsWithErrors, &model.RegWithError{
				Registration: zpaStudentRegs[i],
				Error:        e,
			})
		}
	}

	err = p.dbClient.SetRegsWithErrors(ctx, regsWithErrors)
	if err != nil {
		return 0, nil, err
	}

	return len(zpaStudentRegs) - len(regsWithErrors), regsWithErrors, nil
}

func noError(zpaStudentRegError *model.ZPAStudentRegError) bool {
	return len(zpaStudentRegError.Semester) == 0 &&
		len(zpaStudentRegError.AnCode) == 0 &&
		len(zpaStudentRegError.Exam) == 0 &&
		len(zpaStudentRegError.Mtknr) == 0 &&
		len(zpaStudentRegError.Program) == 0
}
