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
	Mtknr string `bson:"MTKNR"`
	// PrimussAncode is the Primuss ancode (per-program namespace), imported verbatim
	// from the Primuss XLSX "AnCode" column. It equals the ZPA ancode only for FK07
	// exams; for MUC.DAI/external exams it differs. The bson tag stays "AnCode" so the
	// stored studentregs_<program> collections keep decoding unchanged.
	PrimussAncode int    `bson:"AnCode"`
	Program       string `bson:"Stg"`
	Group         string `bson:"Stgru"`
	Name          string `bson:"name"`
	Presence      string `bson:"praesenz_fern"`
}

type Conflicts struct {
	AnCode     int         `json:"ancode"`
	Module     string      `json:"module"`
	MainExamer string      `json:"mainExamer"`
	Conflicts  []*Conflict `json:"conflicts"`
}

type Conflict struct {
	AnCode        int `json:"ancode"`
	NumberOfStuds int `json:"numberOfStuds"`
}
