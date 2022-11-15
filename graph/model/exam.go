package model

type RemovedPrimussExam struct {
	AnCode  int    `bson:"AnCode"`
	Program string `bson:"Stg"`
}

type Exam struct {
	Semester            string               `json:"semester"`
	AnCode              int                  `json:"ancode"`
	Module              string               `json:"module"`
	MainExamer          *Teacher             `json:"main_examer"`
	MainExamerName      string               `json:"main_examer_name"`
	MainExamerID        int                  `json:"main_examer_id"`
	ExamType            string               `json:"exam_type"`
	Duration            int                  `json:"duration"`
	IsRepeaterExam      bool                 `json:"is_repeater_exam"`
	ZpaGroups           []string             `json:"groups"`
	RemovedPrimussExams []RemovedPrimussExam `json:"removedPrimussExams"`
	RegisteredExams     []*RegisteredExam    `json:"registeredExams"`
}
