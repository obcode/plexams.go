package model

type PlanEntry struct {
	DayNumber     int `json:"dayNumber"`
	SlotNumber    int `json:"slotNumber"`
	ExamGroupCode int `json:"examGroupCode"`
}
