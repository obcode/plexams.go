package model

import "time"

// InvigilatorConstraints are the per-invigilator constraints kept in the DB and
// edited via the GUI, separate from the ZPA-sourced invigilator_requirements
// (which is overwritten on every ZPA pull). They are merged on top of the ZPA
// requirements when building an invigilator.
type InvigilatorConstraints struct {
	TeacherID        int                       `json:"teacherID"`
	IsNotInvigilator bool                      `json:"isNotInvigilator"`
	ExcludedDates    []time.Time               `json:"excludedDates"`
	TimeWindows      []*InvigilationTimeWindow `json:"timeWindows"`
}
