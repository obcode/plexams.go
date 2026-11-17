package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
)

// Queries hands the generated query set to plexams/inquiry, bound to the transaction in
// ctx if there is one.
//
// A deliberate exception to "db methods hide sqlc": every inquiry query names its
// semester explicitly, and the portal must never fall back to the process-global
// active semester this type carries. plexams/inquiry narrows the set to the methods it
// needs with an interface of its own, so it cannot reach anything else.
func (db *PG) Queries(ctx context.Context) *sqlc.Queries { return db.q(ctx) }

// PreplanPayload is what an examiner states about one EXaHM/SEB exam. Stored as jsonb
// in inquiry_submission_revision.payload (format_version 1).
type PreplanPayload struct {
	ExamKind         string   `json:"examKind"`
	Module           string   `json:"module"`
	Programs         []string `json:"programs"`
	ExpectedStudents int      `json:"expectedStudents"`
	DurationMin      *int     `json:"durationMin,omitempty"`
	Remark           string   `json:"remark"`
}

// ActivityChange is one changed field of a portal entry, as inquiry_activity.changes
// stores it.
type ActivityChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}
