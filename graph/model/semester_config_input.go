package model

import "time"

// SemesterConfigInput holds the raw, editable per-semester configuration — the
// values that used to live in <semester>.yaml. It is the source of truth from
// which the derived SemesterConfig (days, slots, MUC.DAI slots, forbidden slots)
// is computed in deriveSemesterConfig. It is persisted in the per-semester DB
// (collection semester_config_input).
type SemesterConfigInput struct {
	// From is the start of the planning period; day 1 = from. Exams of other
	// faculties may have earlier dates (or a negative day number); there is no
	// pre-period and no check for that.
	From  time.Time `json:"from" bson:"from"`
	Until time.Time `json:"until" bson:"until"`
	// Slots are the daily slot start times as "HH:MM".
	Slots []string `json:"slots" bson:"slots"`
	// ForbiddenDays are full days inside the planning window on which no exams may
	// be scheduled.
	ForbiddenDays []time.Time `json:"forbiddenDays,omitempty" bson:"forbiddenDays,omitempty"`
	// MucDaiSlots are the slots reserved for MUC.DAI exams, as absolute
	// [dayNumber, slotNumber] pairs (day 1 = from).
	MucDaiSlots [][]int `json:"mucDaiSlots,omitempty" bson:"mucDaiSlots,omitempty"`
	Emails      *Emails `json:"emails" bson:"emails"`
}
