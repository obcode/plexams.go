package model

type NTAExam struct {
	Semester   string `bson:"semester"`
	AnCode     string `bson:"anCode"`
	Module     string `bson:"module"`
	MainExamer string `bson:"mainExamer"`
}

type NTA struct {
	Name                 string     `bson:"name"`
	Mtknr                string     `bson:"mtknr"`
	Compensation         string     `bson:"compensation"`
	DeltaDurationPercent int        `bson:"deltaDurationPercent"`
	NeedsRoomAlone       bool       `bson:"needsRoomAlone"`
	Program              string     `bson:"program"`
	From                 string     `bson:"from"`
	Until                string     `bson:"until"`
	LastSemester         *string    `bson:"lastSemester"`
	Exams                []*NTAExam `bson:"exams"`
}

func NtaInputToNta(ntaInput NTAInput) *NTA {
	return &NTA{
		Name:                 ntaInput.Name,
		Mtknr:                ntaInput.Mtknr,
		Compensation:         ntaInput.Compensation,
		DeltaDurationPercent: ntaInput.DeltaDurationPercent,
		NeedsRoomAlone:       ntaInput.NeedsRoomAlone,
		Program:              ntaInput.Program,
		From:                 ntaInput.From,
		Until:                ntaInput.Until,
		LastSemester:         nil,
		Exams:                []*NTAExam{},
	}
}
