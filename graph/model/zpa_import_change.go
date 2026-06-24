package model

import "time"

// ZPAImportChange is the recorded diff of the most recent ZPA import of one kind
// (teachers / exams / invigilator requirements) against the DB state before it.
type ZPAImportChange struct {
	Kind    string                  `json:"kind"`
	Time    time.Time               `json:"time"`
	Added   int                     `json:"added"`
	Changed int                     `json:"changed"`
	Removed int                     `json:"removed"`
	Entries []*ZPAImportChangeEntry `json:"entries"`
}

// ZPAImportChangeEntry is one added / removed / changed entry. Fields is only set
// for changed entries.
type ZPAImportChangeEntry struct {
	Type   string                  `json:"type"` // "added" | "removed" | "changed"
	Name   string                  `json:"name"`
	Fields []*ZPAImportFieldChange `json:"fields,omitempty"`
}

// ZPAImportFieldChange is a single field that changed (old → new).
type ZPAImportFieldChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}
