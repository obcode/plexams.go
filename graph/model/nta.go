package model

import "strings"

type NTA struct {
	Name                 string  `bson:"name"`
	Email                *string `bson:"email"`
	Mtknr                string  `bson:"mtknr"`
	Compensation         string  `bson:"compensation"`
	DeltaDurationPercent int     `bson:"deltaDurationPercent"`
	NeedsRoomAlone       bool    `bson:"needsRoomAlone"`
	NeedsHardware        bool    `bson:"needsHardware"`
	Program              string  `bson:"program"`
	From                 string  `bson:"from"`
	Until                string  `bson:"until"`
	LastSemester         *string `bson:"lastSemester"`
	Deactivated          bool    `bson:"deactivated"`
}

// compensationCleaner replaces newlines and tabs with a single space so the
// compensation string stays on one line.
var compensationCleaner = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")

func NtaInputToNta(ntaInput NTAInput) *NTA {
	return &NTA{
		Name:                 ntaInput.Name,
		Email:                ntaInput.Email,
		Mtknr:                ntaInput.Mtknr,
		Compensation:         compensationCleaner.Replace(ntaInput.Compensation),
		DeltaDurationPercent: ntaInput.DeltaDurationPercent,
		NeedsRoomAlone:       ntaInput.NeedsRoomAlone,
		NeedsHardware:        ntaInput.NeedsHardware,
		Program:              ntaInput.Program,
		From:                 ntaInput.From,
		Until:                ntaInput.Until,
		LastSemester:         nil,
	}
}
