package zpa

import (
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

func (zpa *ZPA) GetExams() []*model.ZPAExam {
	return zpa.exams
}

func (zpa *ZPA) getExams() error {
	err := zpa.get(fmt.Sprintf("exams?semester=%s&all=true", zpa.semester), &zpa.exams)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	return nil
}
