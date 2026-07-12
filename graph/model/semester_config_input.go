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
	// StartTimes are the allowed daily exam start times as "HH:MM".
	StartTimes []string `json:"startTimes" bson:"startTimes"`
	// ForbiddenDays are full days inside the planning window on which no exams may
	// be scheduled.
	ForbiddenDays []time.Time `json:"forbiddenDays,omitempty" bson:"forbiddenDays,omitempty"`
	// MucDaiAllowedTimes are the absolute start times allowed for MUC.DAI exams
	// (currently "morning vs afternoon"; will become allowed/forbidden times).
	MucDaiAllowedTimes []time.Time `json:"mucDaiAllowedTimes,omitempty" bson:"mucDaiAllowedTimes,omitempty"`
	Emails             *Emails     `json:"emails" bson:"emails"`
	// ExamGapMinutes is the travel/break buffer a student needs between two of their
	// consecutive exams (nil = use the built-in default). See defaultExamGapMinutes.
	ExamGapMinutes *int `json:"examGapMinutes,omitempty" bson:"examGapMinutes,omitempty"`
	// TimelagMin is the minimum turnaround (minutes) between two uses of a room /
	// between two invigilations (nil = use the built-in default).
	TimelagMin *int `json:"timelagMin,omitempty" bson:"timelagMin,omitempty"`
	// NotTooCloseMinutes is the threshold (minutes, same day) below which two of a
	// student's exams are flagged "too close" (nil = use the built-in default 120).
	NotTooCloseMinutes *int `json:"notTooCloseMinutes,omitempty" bson:"notTooCloseMinutes,omitempty"`
	// CrossCampusGapMinutes is the end-to-start travel buffer a student needs between two
	// exams at DIFFERENT campuses (nil = use the built-in default). See
	// defaultCrossCampusGapMinutes.
	CrossCampusGapMinutes *int `json:"crossCampusGapMinutes,omitempty" bson:"crossCampusGapMinutes,omitempty"`
	// MaxSeatsPerSlot limits COMBINING exams at the same start time: independent exams may
	// share a start time only while their combined registrations stay within this cap (so a
	// slot stays roomable). A single exam (or sameSlot unit) is NOT limited by it — every exam
	// must be schedulable, even with more registrations than the cap, in which case it occupies
	// its slot alone. nil or 0 = no limit.
	MaxSeatsPerSlot *int `json:"maxSeatsPerSlot,omitempty" bson:"maxSeatsPerSlot,omitempty"`
}
