// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

type AnCode struct {
	AnCode int `json:"anCode"`
}

type ConnectedExam struct {
	ZpaExam      *ZPAExam       `json:"zpaExam"`
	PrimussExams []*PrimussExam `json:"primussExams"`
}

type PrimussExamByProgram struct {
	Program string         `json:"program"`
	Exams   []*PrimussExam `json:"exams"`
}

type PrimussExamInput struct {
	AnCode  int    `json:"anCode"`
	Program string `json:"program"`
}

type Semester struct {
	ID string `json:"id"`
}

type ZPAExamsForType struct {
	Type  string     `json:"type"`
	Exams []*ZPAExam `json:"exams"`
}
