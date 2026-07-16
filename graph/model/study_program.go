package model

// StudyProgram (Studiengang) is a first-class, semester-independent entity. It
// lives in the global "plexams" database and is used e.g. by the SEB/EXaHM
// pre-planning to foresee program overlaps before Primuss data exists. Until now
// programs were only strings derived from primuss collections / config.
type StudyProgram struct {
	// Shortname (Kürzel, e.g. "IF") is the unique key.
	Shortname string `json:"shortname" bson:"shortname"`
	Name      string `json:"name" bson:"name"`
	// Degree, e.g. "Bachelor" / "Master" (optional).
	Degree *string `json:"degree,omitempty" bson:"degree,omitempty"`
	// Category groups the program by origin: "fk07" | "joint" | "misc".
	Category string `json:"category" bson:"category"`
	Active   bool   `json:"active" bson:"active"`
	// Retired marks a discontinued program. A retired fk07 program is treated as
	// an "old program" (no longer planned, but still relevant for old exams).
	Retired bool `json:"retired" bson:"retired"`
	// ExternalExamsBase is the base ancode for external (e.g. joint-program) exams of
	// this program: the local ZPA ancode is base + primussAncode. Only relevant for
	// programs whose exams are imported externally (joint/misc).
	ExternalExamsBase *int `json:"externalExamsBase,omitempty" bson:"externalExamsBase,omitempty"`
	// JointFaculty names the joint Studienfakultät this program belongs to (e.g.
	// "MUC.DAI" | "MUC.HEALTH"). Set exactly for category "joint" programs; nil
	// otherwise. Used to group joint programs for import/display.
	JointFaculty *string `json:"jointFaculty,omitempty" bson:"jointFaculty,omitempty"`
}
