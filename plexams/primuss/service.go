// Package primuss imports the Primuss "Sammellisten" data from the XLSX files (student
// registrations, exams, planning counts, conflicts) into the raw per-program collections,
// with change detection on re-import. It depends only on a small DB interface (satisfied
// by *db.DB) and pure XLSX parsing; the cross-domain orchestration around an import
// (marking planning conditions, mapping changed Primuss ancodes to ZPA exams, update
// emails) stays in the plexams package.
package primuss

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

// DB is the persistence the Primuss import needs; *db.DB satisfies it.
type DB interface {
	RawCollection(ctx context.Context, name string) ([]bson.M, error)
	ReplaceRawCollection(ctx context.Context, name string, docs []bson.M) (int, error)
}

// Service imports Primuss XLSX data.
type Service struct {
	db DB
}

// New builds a Primuss import service.
func New(db DB) *Service { return &Service{db: db} }
