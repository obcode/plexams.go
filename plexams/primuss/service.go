// Package primuss imports the Primuss "Sammellisten" data from the XLSX files (student
// registrations, exams, planning counts, conflicts) into typed per-program tables,
// with change detection on re-import. It depends only on a small DB interface (satisfied
// by *db.DB) and pure XLSX parsing; the cross-domain orchestration around an import
// (marking planning conditions, mapping changed Primuss ancodes to ZPA exams, update
// emails) stays in the plexams package.
package primuss

import (
	"context"

	"github.com/obcode/plexams.go/db"
)

// DB is the persistence the Primuss import needs. Both *db.DB and *db.PG
// satisfy it; the Mongo side implements it as a thin adapter over the raw
// collections, which is where the untyped round-trip now ends.
type DB interface {
	PrimussStudentRegRows(ctx context.Context, program string) ([]db.PrimussStudentRegRow, error)
	ReplacePrimussStudentRegs(ctx context.Context, program string, rows []db.PrimussStudentRegRow) (int, error)
	ReplacePrimussExams(ctx context.Context, program string, rows []db.PrimussExamRow) (int, error)
	ReplacePrimussCounts(ctx context.Context, program string, rows []db.PrimussCountRow) (int, error)
	ReplacePrimussConflicts(ctx context.Context, program string, rows []db.PrimussConflictRow) (int, error)
}

// Service imports Primuss XLSX data.
type Service struct {
	db DB
}

// New builds a Primuss import service.
func New(db DB) *Service { return &Service{db: db} }
