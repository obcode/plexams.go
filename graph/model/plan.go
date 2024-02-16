package model

type PlanEntry struct {
	DayNumber  int  `json:"dayNumber"`
	SlotNumber int  `json:"slotNumber"`
	Ancode     int  `json:"ancode"`
	Locked     bool `json:"locked"`
}
