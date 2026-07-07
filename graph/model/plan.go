package model

import "time"

// PlanEntry is one exam's placement. The absolute Starttime is the persisted source
// of truth. External marks an entry placed by another faculty (kept on a plan reset).
type PlanEntry struct {
	// Starttime is the absolute start time (nil = not planned). For our own exams it
	// equals the chosen slot's start time; for external exams it is the other
	// faculty's time (which may lie outside our exam period).
	Starttime *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	Ancode    int        `json:"ancode" bson:"ancode"`
	Locked    bool       `json:"locked" bson:"locked"`
	// PhaseFixed marks an entry fixed by the EXaHM/SEB room phase (phase A) — distinct
	// from Locked (the user's explicit manual lock). Phase B treats it as immovable.
	PhaseFixed bool `json:"phaseFixed" bson:"phasefixed"`
	// External marks an entry whose time was set by another faculty (not planned by
	// us). Such entries are preserved when the generated plan is reset.
	External bool `json:"external" bson:"external"`
}

// IsPlanned reports whether the entry has a start time (placed somewhere, inside or
// outside our exam period).
func (pe *PlanEntry) IsPlanned() bool {
	return pe.Starttime != nil
}
