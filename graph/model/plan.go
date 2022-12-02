package model

import "time"

type PlanEntry struct {
	DayNumber     int `json:"dayNumber"`
	SlotNumber    int `json:"slotNumber"`
	ExamGroupCode int `json:"examGroupCode"`
}

type PlanAncodeEntry struct {
	DayNumber  int `json:"dayNumber"`
	SlotNumber int `json:"slotNumber"`
	Ancode     int `json:"ancode"`
}

type PlannedExam struct {
	Ancode     int
	Module     string
	MainExamer string
	DateTime   *time.Time
}
