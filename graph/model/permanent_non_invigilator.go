package model

// PermanentNonInvigilator is a teacher who never does invigilation duty again
// (e.g. retired). Unlike the per-semester invigilator constraints, this lives in
// the global "plexams" database and therefore carries over from semester to
// semester. It always implies isNotInvigilator.
type PermanentNonInvigilator struct {
	TeacherID int `json:"teacherID"`
	// Name is a denormalized display name, so the entry stays readable even when
	// the teacher has left the FK07 invigilator pool (then they are no longer in
	// invigilatorCandidates and the GUI could not resolve a name).
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
