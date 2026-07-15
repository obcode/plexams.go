package model

// PermanentNonInvigilator is a teacher who does no invigilation duty (e.g.
// retired, or a role/leave-bound exemption like Präsident/Dekanin/Mutterschutz).
// Unlike the per-semester invigilator constraints, this lives in the global
// "plexams" database and therefore carries over from semester to semester. It
// always implies isNotInvigilator for the semesters it applies to.
//
// ValidFrom/ValidUntil bound the semesters the exemption applies to (inclusive,
// semester labels like "2026-SS"). Either may be nil for an open-ended bound: a
// fully open range (both nil) is a truly permanent exemption; a set ValidUntil
// retires the exemption after that semester without deleting the record, so past
// semesters stay historically correct when they are re-planned.
type PermanentNonInvigilator struct {
	TeacherID int `json:"teacherID"`
	// Name is a denormalized display name, so the entry stays readable even when
	// the teacher has left the FK07 invigilator pool (then they are no longer in
	// invigilatorCandidates and the GUI could not resolve a name).
	Name       string  `json:"name"`
	Reason     string  `json:"reason"`
	ValidFrom  *string `json:"validFrom,omitempty"`
	ValidUntil *string `json:"validUntil,omitempty"`
}
