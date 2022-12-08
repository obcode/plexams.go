package zpa

import (
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (zpa *ZPA) GetExams() []*model.ZPAExam {
	return zpa.exams
}

func (zpa *ZPA) getExams() error {
	err := zpa.get(fmt.Sprintf("exams?semester=%s&all=true", strings.Replace(zpa.semester, " ", "%20", 1)), &zpa.exams)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	return nil
}

func (zpa *ZPA) PostExams(exams []*model.ZPAExamPlan) (string, []byte, error) {
	log.Debug().Int("count", len(exams)).Msg("posting exams to zpa")
	return zpa.post("exam_plan", exams)
}
