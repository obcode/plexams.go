package model

type RemovedPrimussExam struct {
	AnCode  int    `bson:"AnCode"`
	Program string `bson:"Stg"`
}

// type Exam struct {
// 	Semester            string               `json:"semester"`
// 	AnCode              int                  `json:"ancode"`
// 	Module              string               `json:"module"`
// 	MainExamer          *Teacher             `json:"main_examer"`
// 	MainExamerName      string               `json:"main_examer_name"`
// 	MainExamerID        int                  `json:"main_examer_id"`
// 	ExamType            string               `json:"exam_type"`
// 	Duration            int                  `json:"duration"`
// 	IsRepeaterExam      bool                 `json:"is_repeater_exam"`
// 	ZpaGroups           []string             `json:"groups"`
// 	RemovedPrimussExams []RemovedPrimussExam `json:"removedPrimussExams"`
// 	RegisteredExams     []*RegisteredExam    `json:"registeredExams"`
// }

func (exam *ExamToPlan) IsGO() bool {
	for _, primussExam := range exam.Exam.PrimussExams {
		if primussExam.Program == "GO" || primussExam.Program == "GN" || primussExam.Program == "GS" {
			return true
		}
	}
	return false
}

type GeneratedExam struct {
	Ancode           int                    `json:"ancode"`
	ZpaExam          *ZPAExam               `json:"zpaExam"`
	PrimussExams     []*EnhancedPrimussExam `json:"primussExams"`
	Constraints      *Constraints           `json:"constraints,omitempty"`
	Conflicts        []*ZPAConflict         `json:"conflicts"`
	StudentRegsCount int                    `json:"studentRegsCount"`
	Ntas             []*NTA                 `json:"ntas"`
	MaxDuration      int                    `json:"maxDuration"`
}

type PlannedExam struct {
	Ancode           int                    `json:"ancode"`
	ZpaExam          *ZPAExam               `json:"zpaExam"`
	PrimussExams     []*EnhancedPrimussExam `json:"primussExams"`
	Constraints      *Constraints           `json:"constraints,omitempty"`
	Conflicts        []*ZPAConflict         `json:"conflicts"`
	StudentRegsCount int                    `json:"studentRegsCount"`
	Ntas             []*NTA                 `json:"ntas"`
	MaxDuration      int                    `json:"maxDuration"`
	PlanEntry        *PlanEntry             `json:"planEntry,omitempty"`
	PlannedRooms     []*PlannedRoom         `json:"plannedRooms,omitempty"`
}
