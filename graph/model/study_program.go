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
	// Category groups the program by origin: "fk07" | "mucdai" | "misc".
	Category string `json:"category" bson:"category"`
	Active   bool   `json:"active" bson:"active"`
}
