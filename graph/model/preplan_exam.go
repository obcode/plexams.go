package model

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
	// PlannedDayNumber / PlannedSlotNumber assign the pre-exam to a slot (both nil
	// = not yet slotted).
	PlannedDayNumber  *int `json:"plannedDayNumber,omitempty" bson:"planneddaynumber,omitempty"`
	PlannedSlotNumber *int `json:"plannedSlotNumber,omitempty" bson:"plannedslotnumber,omitempty"`
	// IsFixed pins the current slot: it survives a re-run of the automatic
	// assignment (which otherwise re-plans all non-fixed pre-exams).
	IsFixed bool `json:"isFixed" bson:"isfixed"`
	// Ancode is set once the pre-exam is linked to a real ZPA exam (phase 4).
	Ancode *int   `json:"ancode,omitempty" bson:"ancode,omitempty"`
	Notes  string `json:"notes,omitempty" bson:"notes,omitempty"`
	// Constraints captured during pre-planning (carried over to the ZPA exam on
	// linking). In Constraints.SameSlot the ints are PRE-EXAM ids, not ancodes.
	Constraints *Constraints `json:"constraints,omitempty" bson:"constraints,omitempty"`
}
