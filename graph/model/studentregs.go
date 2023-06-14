package model

type StudentRegsForAncode struct {
	Exam        *ZPAExam                          `json:"exam"`
	StudentRegs []*StudentRegsPerAncodeAndProgram `json:"studentRegs"`
}
