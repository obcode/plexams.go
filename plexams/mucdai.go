package plexams

import (
	"context"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) MucdaiExams(ctx context.Context) ([]*model.MucDaiExam, error) {
	mucdaiPrograms := viper.GetStringSlice("mucdaiprograms")

	exams := make([]*model.MucDaiExam, 0)

	for _, program := range mucdaiPrograms {
		examsForProgram, err := p.MucDaiExamsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get mucdai exams for program")
		} else {
			exams = append(exams, examsForProgram...)
		}
	}

	return exams, nil
}

func (p *Plexams) MucDaiExamsForProgram(ctx context.Context, program string) ([]*model.MucDaiExam, error) {
	exams, err := p.dbClient.MucDaiExamsForProgram(ctx, program)
	if err != nil {
		return nil, err
	}
	mucdaiExams := make([]*model.MucDaiExam, 0, len(exams))
	for _, exam := range exams {
		mucdaiExams = append(mucdaiExams, p.mkMucdaiExam(exam))
	}

	return mucdaiExams, nil
}

func (p *Plexams) MucDaiExam(ctx context.Context, program string, ancode int) (*model.MucDaiExam, error) {
	exam, err := p.dbClient.MucDaiExam(ctx, program, ancode)
	if err != nil {
		return nil, err
	}
	return p.mkMucdaiExam(exam), nil
}

func (p *Plexams) mkMucdaiExam(mucdaiExam *db.MucDaiExam) *model.MucDaiExam {
	isRepeaterExam := mucdaiExam.IsRepeaterExam == "x"

	return &model.MucDaiExam{
		PrimussAncode:  mucdaiExam.PrimussAncode,
		Module:         mucdaiExam.Module,
		MainExamer:     mucdaiExam.MainExamer,
		ExamType:       mucdaiExam.ExamType,
		Duration:       mucdaiExam.Duration,
		IsRepeaterExam: isRepeaterExam,
		Program:        mucdaiExam.Program,
		PlannedBy:      mucdaiExam.Planer,
	}
}

func (p *Plexams) AddMucDaiExam(ctx context.Context, zpaAncode int, mucdaiExam *model.MucDaiExam) (*model.ZPAExam, error) {
	zpaExam := &model.ZPAExam{
		ZpaID:          0,
		Semester:       p.semester,
		AnCode:         zpaAncode,
		Module:         mucdaiExam.Module,
		MainExamer:     mucdaiExam.MainExamer,
		MainExamerID:   0,
		ExamType:       mucdaiExam.ExamType,
		ExamTypeFull:   "",
		Date:           "",
		Starttime:      "",
		Duration:       mucdaiExam.Duration,
		IsRepeaterExam: mucdaiExam.IsRepeaterExam,
		Groups:         []string{},
		PrimussAncodes: []model.ZPAPrimussAncodes{{
			Program: mucdaiExam.Program,
			Ancode:  mucdaiExam.PrimussAncode,
		}},
	}

	err := p.dbClient.AddNonZpaExam(ctx, zpaExam)

	return zpaExam, err
}
