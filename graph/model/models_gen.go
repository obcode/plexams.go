// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

type PrimussExamByGroup struct {
	Group string         `json:"group"`
	Exams []*PrimussExam `json:"exams"`
}

type Semester struct {
	ID string `json:"id"`
}

type ZPAExamsForType struct {
	Type  string     `json:"type"`
	Exams []*ZPAExam `json:"exams"`
}
