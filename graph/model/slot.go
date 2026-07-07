package model

import "time"

// Slot is one candidate slot of the internal exam grid. DayNumber/SlotNumber are the
// internal grid coordinates (used throughout the planning logic); the API exposes only
// the absolute Starttime.
type Slot struct {
	DayNumber  int       `json:"-"`
	SlotNumber int       `json:"-"`
	Starttime  time.Time `json:"starttime"`
}
