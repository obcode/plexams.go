package model

type PrimussExam struct {
	AnCode     int    `bson:"AnCode"`
	Module     string `bson:"Titel"`
	MainExamer string `bson:"pruefer"`
	Program    string `bson:"Stg"`
	ExamType   string `bson:"sonst"`
	Presence   string `bson:"ist_praesenz"`
}

type StudentReg struct {
	Mtknr    string `bson:"MTKNR"`
	AnCode   int    `bson:"AnCode"`
	Program  string `bson:"Stg"`
	Group    string `bson:"Stgru"`
	Name     string `bson:"name"`
	Presence string `bson:"praesenz_fern"`
}

type Conflicts struct {
	AnCode     int        `json:"anCode"`
	Module     string     `json:"module"`
	MainExamer string     `json:"mainExamer"`
	Conflicts  []Conflict `json:"conflicts"`
}

type Conflict struct {
	AnCode        int `json:"anCode"`
	NumberOfStuds int `json:"numberOfStuds"`
}
