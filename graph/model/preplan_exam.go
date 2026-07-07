package model

import "time"

// PreplanExam is a manually entered pseudo-exam for the SEB/EXaHM pre-planning of the
// (next) semester, captured before the ZPA exam list and Primuss data exist. It
// lives in the semester's own database (collection preplan_exams). Once the real ZPA
// exams are imported, a PreplanExam is linked to its ancode (phase 4).
type PreplanExam struct {
	// ID is a synthetic per-semester key (assigned on creation).
	ID       int    `json:"id" bson:"id"`
	ExamKind string `json:"examKind" bson:"examkind"` // "EXaHM" | "SEB"
	// ExamerID links to a Teacher; ExamerName is a snapshot for display.
	ExamerID         int      `json:"examerID" bson:"examerid"`
	ExamerName       string   `json:"examerName" bson:"examername"`
	Module           string   `json:"module" bson:"module"`
	Programs         []string `json:"programs" bson:"programs"` // StudyProgram shortnames
	ExpectedStudents int      `json:"expectedStudents" bson:"expectedstudents"`
	Duration         *int     `json:"duration,omitempty" bson:"duration,omitempty"`
	// PlannedStarttime is the absolute start time the pre-exam is assigned to (nil = not
	// yet placed). This is the source of truth — there are no day/slot ordinals; a time
	// outside the (next) semester's period simply has no matching booked slot.
	PlannedStarttime *time.Time `json:"plannedStarttime,omitempty" bson:"plannedstarttime,omitempty"`
	// IsFixed pins the current slot: it survives a re-run of the automatic
	// assignment (which otherwise re-plans all non-fixed pre-exams).
	IsFixed bool `json:"isFixed" bson:"isfixed"`
	// NotSameSlot holds PRE-EXAM ids that should NOT run at the same time as this one
	// (same students, even when the study program does not show it). Kept symmetric.
	// Soft: the solver spreads them apart (different days, else max slot distance).
	NotSameSlot []int `json:"notSameSlot,omitempty" bson:"notsameslot,omitempty"`
	// CanShareSlot holds PRE-EXAM ids that MAY run at the same time / right after this
	// one despite sharing a study program (no common students). Kept symmetric. It
	// cancels the program-based spreading penalty for that pair.
	CanShareSlot []int `json:"canShareSlot,omitempty" bson:"canshareslot,omitempty"`
	// Ancode is set once the pre-exam is linked to a real ZPA exam (phase 4).
	Ancode *int   `json:"ancode,omitempty" bson:"ancode,omitempty"`
	Notes  string `json:"notes,omitempty" bson:"notes,omitempty"`
	// Constraints captured during pre-planning (carried over to the ZPA exam on
	// linking). In Constraints.SameSlot the ints are PRE-EXAM ids, not ancodes.
	Constraints *Constraints `json:"constraints,omitempty" bson:"constraints,omitempty"`
}
