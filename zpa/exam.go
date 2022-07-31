package zpa

import "fmt"

type Exam struct {
	Semester       string   `json:"semester"`
	AnCode         int      `json:"anCode"`
	Module         string   `json:"module"`
	MainExamer     string   `json:"main_examer"`
	MainExamerID   int      `json:"main_examer_id"`
	ExamType       string   `json:"exam_type"`
	Duration       int      `json:"duration"`
	IsRepeaterExam bool     `json:"is_repeater_exam"`
	Groups         []string `json:"groups"`
}

func (zpa *ZPA) GetExams() []*Exam {
	return zpa.exams
}

func (zpa *ZPA) getExams() error {
	err := zpa.get(fmt.Sprintf("exams?semester=%s", zpa.semester), &zpa.exams)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	return nil
}
