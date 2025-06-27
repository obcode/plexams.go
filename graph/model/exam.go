package model

type RemovedPrimussExam struct {
	AnCode  int    `bson:"AnCode"`
	Program string `bson:"Stg"`
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
