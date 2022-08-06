package model

type ZPAExam struct {
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
