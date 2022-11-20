package plexams

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AddNta(ctx context.Context, input model.NTAInput) (*model.NTA, error) {
	return p.dbClient.AddNta(ctx, model.NtaInputToNta(input))
}

func (p *Plexams) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return p.dbClient.Ntas(ctx)
}

func (p *Plexams) NtasWithRegs(ctx context.Context) ([]*model.NTAWithRegs, error) {
	return p.dbClient.NtasWithRegs(ctx)
}

func (p *Plexams) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return p.dbClient.Nta(ctx, mtknr)
}

func (p *Plexams) PrepareNta() error {
	ctx := context.Background()
	// get NTAs
	ntas, err := p.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get NTAs")
		return err
	}

	// get StudentRegs
	studentRegs, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs")
		return err
	}

	// merge
	ntaWithRegs := make([]*model.NTAWithRegs, 0)
	for _, nta := range ntas {
		for _, studentReg := range studentRegs {
			if nta.Mtknr == studentReg.Student.Mtknr {
				ntaWithRegs = append(ntaWithRegs, &model.NTAWithRegs{
					Nta:  nta,
					Regs: studentReg,
				})
				break
			}
		}
	}

	err = p.dbClient.SaveSemesterNTAs(ctx, ntaWithRegs)
	if err != nil {
		log.Error().Err(err).Msg("cannot save ntas for semester")
		return err
	}

	return nil
}

func (p *Plexams) NtasWithRegsByTeacher(ctx context.Context) ([]*model.NTAWithRegsByExamAndTeacher, error) {
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get NTAs with regs")
		return nil, err
	}

	ancodesMap := make(map[int][]*model.NTAWithRegs)
	for _, nta := range ntas {
		for _, ancode := range nta.Regs.Ancodes {
			entry, ok := ancodesMap[ancode]
			if ok {
				entry = append(entry, nta)
			} else {
				entry = []*model.NTAWithRegs{nta}
			}
			ancodesMap[ancode] = entry
		}
	}

	mainExamerMap := make(map[int][]*model.ZPAExam)
	for ancode := range ancodesMap {
		exam, err := p.GetZpaExamByAncode(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam with ancode")
			return nil, err
		}

		exams, ok := mainExamerMap[exam.MainExamerID]
		if ok {
			exams = append(exams, exam)
		} else {
			exams = []*model.ZPAExam{exam}
		}
		mainExamerMap[exam.MainExamerID] = exams
	}

	// sort teacher
	teacherMap := make(map[string]*model.Teacher)
	teacherNames := make([]string, 0, len(mainExamerMap))
	for mainExamerID := range mainExamerMap {
		teacher, err := p.GetTeacher(ctx, mainExamerID)
		if err != nil {
			log.Error().Err(err).Int("mainExamerID", mainExamerID).Msg("cannot get teacher with mainExamerID")
			return nil, err
		}
		teacherMap[teacher.Shortname] = teacher
		teacherNames = append(teacherNames, teacher.Shortname)
	}
	sort.Strings(teacherNames)

	result := make([]*model.NTAWithRegsByExamAndTeacher, 0, len(mainExamerMap))
	for _, teacherName := range teacherNames {
		teacher := teacherMap[teacherName]
		exams := mainExamerMap[teacher.ID]
		examsResult := make([]*model.NTAWithRegsByExam, 0)
		for _, exam := range exams {
			constraint, err := p.ConstraintForAncode(ctx, exam.AnCode)
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot get constraint for ancode")
			}
			if constraint != nil && constraint.NotPlannedByMe {
				break
			}
			examsResult = append(examsResult, &model.NTAWithRegsByExam{
				Exam: exam,
				Ntas: ancodesMap[exam.AnCode],
			})
		}

		if len(examsResult) > 0 {
			result = append(result, &model.NTAWithRegsByExamAndTeacher{
				Teacher: teacher,
				Exams:   examsResult,
			})
		}
	}

	return result, err
}
