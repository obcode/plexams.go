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

// InSlot reports whether this entry is placed in one of our real exam slots. It is
// false for an external exam whose time lies outside our exam period: such an entry has
// no slot (DayNumber/SlotNumber 0) and carries only ExternalTime. Use this instead of
// comparing DayNumber/SlotNumber to the 0/0 sentinel by hand.
func (pe *PlanEntry) InSlot() bool {
	return pe.DayNumber > 0 && pe.SlotNumber > 0
}
