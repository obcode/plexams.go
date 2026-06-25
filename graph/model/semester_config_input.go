package model

import "time"

// SemesterConfigInput holds the raw, editable per-semester configuration — the
// values that used to live in <semester>.yaml under semesterConfig/goslots. It
// is the source of truth from which the derived SemesterConfig (days, slots,
// go-slots, forbidden slots) is computed in deriveSemesterConfig. It is
// persisted in the per-semester DB (collection semester_config_input).
type SemesterConfigInput struct {
	// From is the start of the planning period; day 1 = from. Exams of other
	// faculties may have earlier dates (or a negative day number); there is no
	// pre-period and no check for that.
	From  time.Time `json:"from" bson:"from"`
	Until time.Time `json:"until" bson:"until"`
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
