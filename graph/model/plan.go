package model

import "time"

type PlanEntry struct {
	DayNumber    int        `json:"dayNumber"`
	SlotNumber   int        `json:"slotNumber"`
	ExternalTime *time.Time `json:"externalTime"`
	Ancode       int        `json:"ancode"`
	Locked       bool       `json:"locked"`
	// PhaseFixed marks an entry fixed by the EXaHM/SEB room phase (phase A) — distinct
	// from Locked (the user's explicit manual lock). Phase B treats it as immovable.
	PhaseFixed bool `json:"phaseFixed"`
}
