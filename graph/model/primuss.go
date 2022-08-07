package model

type PrimussExam struct {
	AnCode     int    `json:"AnCode"`
	Module     string `bson:"Titel"`
	MainExamer string `bson:"pruefer"`
	Group      string `bson:"Stg"`
	ExamType   string `bson:"sonst"`
	Presence   string `bson:"ist_praesenz"`
}
