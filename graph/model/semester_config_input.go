package model

import "time"

// SemesterConfigInput holds the raw, editable per-semester configuration — the
// values that used to live in <semester>.yaml under semesterConfig/goslots. It
// is the source of truth from which the derived SemesterConfig (days, slots,
// go-slots, forbidden slots) is computed in deriveSemesterConfig. It is
// persisted in the per-semester DB (collection semester_config_input).
type SemesterConfigInput struct {
	From     time.Time `json:"from" bson:"from"`
	FromFk07 time.Time `json:"fromFK07" bson:"fromFK07"`
	Until    time.Time `json:"until" bson:"until"`
	// DayNumberStart == "from" numbers days from `from` (legacy plans); empty (or
	// anything else) numbers from fromFK07.
	DayNumberStart string `json:"dayNumberStart,omitempty" bson:"dayNumberStart,omitempty"`
	// Slots are the daily slot start times as "HH:MM".
	Slots  []string  `json:"slots" bson:"slots"`
	GoDay0 time.Time `json:"goDay0" bson:"goDay0"`
	// ForbiddenDays are full days inside the planning window on which no exams may
	// be scheduled.
	ForbiddenDays []time.Time `json:"forbiddenDays,omitempty" bson:"forbiddenDays,omitempty"`
	// GoSlots are [dayOffsetFromGoDay0, slotNumber] pairs (the YAML top-level
	// goslots block).
	GoSlots [][]int `json:"goSlots,omitempty" bson:"goSlots,omitempty"`
	Emails  *Emails `json:"emails" bson:"emails"`
}
