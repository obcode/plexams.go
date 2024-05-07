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

	exams := make([]*db.MucDaiExam, 0)

	for _, program := range mucdaiPrograms {
		examsForProgram, err := p.dbClient.MucDaiExamsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get mucdai exams for program")
		} else {
			exams = append(exams, examsForProgram...)
		}
	}

	mucdaiExams := make([]*model.MucDaiExam, 0, len(exams))
	for _, exam := range exams {
		mucdaiExams = append(mucdaiExams, p.mkMucdaiExam(exam))
	}

	return mucdaiExams, nil
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
