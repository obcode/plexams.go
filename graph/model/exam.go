package model

type RemovedPrimussExam struct {
	AnCode  int    `bson:"AnCode"`
	Program string `bson:"Stg"`
}

type AssembledExam struct {
	Ancode           int                    `json:"ancode"`
	ZpaExam          *ZPAExam               `json:"zpaExam"`
	PrimussExams     []*EnhancedPrimussExam `json:"primussExams"`
	Constraints      *Constraints           `json:"constraints,omitempty"`
	Conflicts        []*ZPAConflict         `json:"conflicts"`
	StudentRegsCount int                    `json:"studentRegsCount"`
	Ntas             []*NTA                 `json:"ntas"`
	MaxDuration      int                    `json:"maxDuration"`
}

// Ancodes returns the exam's internal/external ancode bundle (bound to the GraphQL
// `ancodes` field). Computed from ZpaExam, so it never drifts from the stored data.
func (e *AssembledExam) Ancodes() Ancodes {
	if e.ZpaExam == nil {
		return Ancodes{ZpaAncode: e.Ancode}
	}
	return e.ZpaExam.Ancodes()
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

// Ancodes returns the exam's internal/external ancode bundle (bound to the GraphQL
// `ancodes` field). Computed from ZpaExam, so it never drifts from the stored data.
func (e *PlannedExam) Ancodes() Ancodes {
	if e.ZpaExam == nil {
		return Ancodes{ZpaAncode: e.Ancode}
	}
	return e.ZpaExam.Ancodes()
}
