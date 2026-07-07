package model

import "time"

// Slot is one candidate start time of the exam grid. The absolute Starttime is the
// only identity; any positional (day/slot) notion is derived locally from the sorted
// config start times where needed, never stored.
type Slot struct {
	Starttime time.Time `json:"starttime"`
}
