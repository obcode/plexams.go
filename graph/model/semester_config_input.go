package model

import "time"

// JointProgramTimes are the absolute start times reserved for one joint study
// program's exams (e.g. DE / GS / ID of MUC.DAI, or a MUC.HEALTH program). Each
// joint program can have its own reserved window; an exam is restricted to the
// intersection of the reserved slots of all joint programs its students belong to.
type JointProgramTimes struct {
	// Program is the study program shortname (Kürzel), e.g. "DE".
	Program string `json:"program" bson:"program"`
	// AllowedTimes are the absolute start times reserved for this program.
	AllowedTimes []time.Time `json:"allowedTimes" bson:"allowedTimes"`
}

// SemesterConfigInput holds the raw, editable per-semester configuration — the
// values that used to live in <semester>.yaml. It is the source of truth from
// which the derived SemesterConfig (days, slots, joint-program slots, forbidden
// slots) is computed in deriveSemesterConfig. It is persisted in the per-semester
// DB (collection semester_config_input).
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
	// JointProgramAllowedTimes are the absolute start times reserved per joint study
	// program (one entry per program, e.g. DE/GS/ID/…). Replaces the former single
	// MucDaiAllowedTimes list.
	JointProgramAllowedTimes []*JointProgramTimes `json:"jointProgramAllowedTimes,omitempty" bson:"jointProgramAllowedTimes,omitempty"`
	// MucDaiAllowedTimes is the LEGACY single MUC.DAI reserved-times list. Kept only
	// so a stored pre-migration config still decodes; on load it seeds every joint
	// program's times when JointProgramAllowedTimes is empty (see loadSemesterConfig).
	MucDaiAllowedTimes []time.Time `json:"-" bson:"mucDaiAllowedTimes,omitempty"`
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
