package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// BackupStatus reports whether the current semester changed since the last full
// ZIP dump was downloaded, so the GUI can prompt the planner to take a backup.
// hasUnsavedChanges is true when there is a change newer than the last dump (or a
// dump was never taken); it is false for an untouched semester.
func (p *Plexams) BackupStatus(ctx context.Context) (*model.BackupStatus, error) {
	meta, err := p.dbClient.GetSemesterMeta(ctx)
	if err != nil {
		return nil, err
	}
	var lastDumpAt *time.Time
	if meta != nil {
		lastDumpAt = meta.LastDumpAt
	}

	lastChangeAt, err := p.dbClient.LatestMutationTime(ctx)
	if err != nil {
		return nil, err
	}

	hasUnsavedChanges := lastChangeAt != nil && (lastDumpAt == nil || lastChangeAt.After(*lastDumpAt))

	return &model.BackupStatus{
		HasUnsavedChanges: hasUnsavedChanges,
		LastDumpAt:        lastDumpAt,
		LastChangeAt:      lastChangeAt,
	}, nil
}
