package model

import "time"

// SyncLogEntry records one external transfer (import from / upload to ZPA, Anny,
// …). The collection keeps the full history since the start of the semester, so
// the GUI can show the last sync per operation and a timeline.
type SyncLogEntry struct {
	Time      time.Time `json:"time"`
	Operation string    `json:"operation"` // stable key, e.g. "zpa-import-exams", "zpa-upload-rooms", "anny-import"
	Label     string    `json:"label"`     // human-readable label
	Direction string    `json:"direction"` // "import" | "upload"
	System    string    `json:"system"`    // "ZPA" | "Anny" | …
	OK        bool      `json:"ok"`
	Summary   string    `json:"summary"`
	// For imports: how many entries were added / changed / removed, and the
	// per-entry detail. Empty / nil for uploads.
	Added   int                `json:"added"`
	Changed int                `json:"changed"`
	Removed int                `json:"removed"`
	Entries []*SyncChangeEntry `json:"entries,omitempty"`
}

// SyncChangeEntry is one added / removed / changed entry of an import. Fields is
// only set for changed entries.
type SyncChangeEntry struct {
	Type   string             `json:"type"` // "added" | "removed" | "changed"
	Name   string             `json:"name"`
	Fields []*SyncFieldChange `json:"fields,omitempty"`
}

// SyncFieldChange is a single field that changed (old → new).
type SyncFieldChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}
