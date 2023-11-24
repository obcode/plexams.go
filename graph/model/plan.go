package model

import "time"

type PlanEntry struct {
	DayNumber     int  `json:"dayNumber"`
	SlotNumber    int  `json:"slotNumber"`
	ExamGroupCode int  `json:"examGroupCode"`
	Locked        bool `json:"locked"`
}

type PlanAncodeEntry struct {
	DayNumber  int  `json:"dayNumber"`
	SlotNumber int  `json:"slotNumber"`
	Ancode     int  `json:"ancode"`
	Locked     bool `json:"locked"`
}

type PlannedExam struct {
	Ancode     int
	Module     string
	MainExamer string
	DateTime   *time.Time
}
