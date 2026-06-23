package model

// PermanentNonInvigilator is a teacher who never does invigilation duty again
// (e.g. retired). Unlike the per-semester invigilator constraints, this lives in
// the global "plexams" database and therefore carries over from semester to
// semester. It always implies isNotInvigilator.
type PermanentNonInvigilator struct {
	TeacherID int    `json:"teacherID"`
	Reason    string `json:"reason"`
}
