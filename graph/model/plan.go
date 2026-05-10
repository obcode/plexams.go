package model

import "time"

type PlanEntry struct {
	DayNumber    int        `json:"dayNumber"`
	SlotNumber   int        `json:"slotNumber"`
	ExternalTime *time.Time `json:"externalTime"`
	Ancode       int        `json:"ancode"`
	Locked       bool       `json:"locked"`
}
