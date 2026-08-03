package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// syncLogFormatVersion is the shape of the per-entry diff stored in
// sync_log.entries -- a nested list of lists, read and written with its row.
const syncLogFormatVersion = 1

// AddMutationLogEntry appends one mutating-operation entry to the semester's
// mutation log.
func (db *PG) AddMutationLogEntry(ctx context.Context, entry *model.MutationLogEntry) error {
	args := entry.Args
	if args == nil {
		args = make([]*model.MutationLogArg, 0)
	}
	blob, err := json.Marshal(args)
	if err != nil {
		log.Error().Err(err).Str("name", entry.Name).Msg("cannot add mutation-log entry")
		return err
	}

	ancodes := entry.Ancodes
	if ancodes == nil {
		ancodes = make([]int, 0)
	}

	err = db.q(ctx).InsertMutationLogEntry(ctx, sqlc.InsertMutationLogEntryParams{
		SemesterID: db.semesterID,
		LoggedAt:   entry.Time,
		Name:       entry.Name,
		Type:       entry.Type,
		UserEmail:  entry.User,
		Args:       blob,
		Ancodes:    ancodes,
		Error:      entry.Error,
		DurationMs: entry.DurationMs,
	})
	if err != nil {
		log.Error().Err(err).Str("name", entry.Name).Msg("cannot add mutation-log entry")
		return err
	}
	return nil
}

// MutationLog returns the mutation log, newest first, filtered by operation
// name, a referenced ancode, argument key/value pairs, the operator and/or a
// time range. limit <= 0 returns all.
//
// The argument filter was Mongo's one $elemMatch; here it is jsonb containment,
// which says the same thing about a list of key/value objects.
func (db *PG) MutationLog(ctx context.Context, opType, name *string, ancode *int,
	argFilters []*model.ArgFilterInput, user *string, since, until *time.Time, limit int,
) ([]*model.MutationLogEntry, error) {
	params := sqlc.ListMutationLogParams{
		SemesterID: db.semesterID,
		OpType:     emptyToNil(opType),
		OpName:     emptyToNil(name),
		UserEmail:  emptyToNil(user),
		Ancode:     ancode,
		Since:      since,
		Until:      until,
		MaxRows:    limit,
	}
	if limit < 0 {
		params.MaxRows = 0
	}

	if filters := argFilterDoc(argFilters); filters != nil {
		blob, err := json.Marshal(filters)
		if err != nil {
			log.Error().Err(err).Msg("cannot encode mutation-log argument filter")
			return nil, err
		}
		params.ArgFilters = blob
	}

	rows, err := db.q(ctx).ListMutationLog(ctx, params)
	if err != nil {
		log.Error().Err(err).Msg("cannot find mutation log")
		return nil, err
	}

	entries := make([]*model.MutationLogEntry, 0, len(rows))
	for _, row := range rows {
		entry := &model.MutationLogEntry{
			Time:       row.LoggedAt,
			Name:       row.Name,
			Type:       row.Type,
			User:       row.UserEmail,
			Ancodes:    row.Ancodes,
			Error:      row.Error,
			DurationMs: row.DurationMs,
			Args:       make([]*model.MutationLogArg, 0),
		}
		if entry.Ancodes == nil {
			entry.Ancodes = make([]int, 0)
		}
		if len(row.Args) > 0 {
			if err := json.Unmarshal(row.Args, &entry.Args); err != nil {
				log.Error().Err(err).Msg("cannot decode mutation log")
				return nil, err
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// argFilterDoc turns the filters into the document the stored args must contain.
// Nil when there is nothing to filter on.
func argFilterDoc(argFilters []*model.ArgFilterInput) []map[string]string {
	filters := make([]map[string]string, 0, len(argFilters))
	for _, af := range argFilters {
		if af == nil {
			continue
		}
		filters = append(filters, map[string]string{"key": af.Key, "value": af.Value})
	}
	if len(filters) == 0 {
		return nil
	}
	return filters
}

// emptyToNil maps the empty string to nil: every one of these filters treated
// "" as "do not filter".
func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// LatestMutationTime returns the timestamp of the most recent mutation-log
// entry, or (nil, nil) when the log is empty (nothing changed yet).
func (db *PG) LatestMutationTime(ctx context.Context) (*time.Time, error) {
	at, err := db.q(ctx).LatestMutationTime(ctx, db.semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get latest mutation-log time")
		return nil, err
	}
	return &at, nil
}

// MutationLogNames returns the distinct operation names present in the log.
func (db *PG) MutationLogNames(ctx context.Context) ([]string, error) {
	names, err := db.q(ctx).ListMutationLogNames(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get distinct mutation-log names")
		return nil, err
	}
	return names, nil
}

// AddSyncLogEntry appends one transfer event to the sync-log history.
func (db *PG) AddSyncLogEntry(ctx context.Context, entry *model.SyncLogEntry) error {
	changes := entry.Entries
	if changes == nil {
		changes = make([]*model.SyncChangeEntry, 0)
	}
	blob, err := json.Marshal(changes)
	if err != nil {
		log.Error().Err(err).Str("operation", entry.Operation).Msg("cannot add sync-log entry")
		return err
	}

	err = db.q(ctx).InsertSyncLogEntry(ctx, sqlc.InsertSyncLogEntryParams{
		SemesterID:    db.semesterID,
		LoggedAt:      entry.Time,
		Operation:     entry.Operation,
		Label:         entry.Label,
		Direction:     entry.Direction,
		System:        entry.System,
		Ok:            entry.OK,
		Summary:       entry.Summary,
		Added:         entry.Added,
		Changed:       entry.Changed,
		Removed:       entry.Removed,
		Entries:       blob,
		FormatVersion: syncLogFormatVersion,
	})
	if err != nil {
		log.Error().Err(err).Str("operation", entry.Operation).Msg("cannot add sync-log entry")
		return err
	}
	return nil
}

// SyncLog returns the transfer history, newest first. limit <= 0 returns all.
func (db *PG) SyncLog(ctx context.Context, limit int) ([]*model.SyncLogEntry, error) {
	if limit < 0 {
		limit = 0
	}
	rows, err := db.q(ctx).ListSyncLog(ctx, sqlc.ListSyncLogParams{
		SemesterID: db.semesterID,
		MaxRows:    limit,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot find sync-log")
		return nil, err
	}

	entries := make([]*model.SyncLogEntry, 0, len(rows))
	for _, row := range rows {
		if row.FormatVersion != syncLogFormatVersion {
			err := fmt.Errorf("sync-log entry was written in format version %d, this binary reads %d",
				row.FormatVersion, syncLogFormatVersion)
			log.Error().Err(err).Msg("cannot decode sync-log")
			return nil, err
		}
		entry := &model.SyncLogEntry{
			Time:      row.LoggedAt,
			Operation: row.Operation,
			Label:     row.Label,
			Direction: row.Direction,
			System:    row.System,
			OK:        row.Ok,
			Summary:   row.Summary,
			Added:     row.Added,
			Changed:   row.Changed,
			Removed:   row.Removed,
		}
		if len(row.Entries) > 0 {
			if err := json.Unmarshal(row.Entries, &entry.Entries); err != nil {
				log.Error().Err(err).Msg("cannot decode sync-log")
				return nil, err
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// PlanningConditionsSet returns the keys of all conditions that are currently
// set (done) for the semester. Presence of the row is the condition holding.
func (db *PG) PlanningConditionsSet(ctx context.Context) ([]string, error) {
	keys, err := db.q(ctx).ListPlanningConditions(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planning conditions")
		return nil, err
	}
	return keys, nil
}

// SetPlanningCondition marks a condition as done (set=true) or removes it
// (set=false). Setting one twice is not an error, as the upserting replace was
// not.
func (db *PG) SetPlanningCondition(ctx context.Context, key string, set bool) error {
	if set {
		if err := db.q(ctx).SetPlanningCondition(ctx, sqlc.SetPlanningConditionParams{
			SemesterID:   db.semesterID,
			ConditionKey: key,
		}); err != nil {
			log.Error().Err(err).Str("key", key).Msg("cannot set planning condition")
			return err
		}
		return nil
	}

	if err := db.q(ctx).UnsetPlanningCondition(ctx, sqlc.UnsetPlanningConditionParams{
		SemesterID:   db.semesterID,
		ConditionKey: key,
	}); err != nil {
		log.Error().Err(err).Str("key", key).Msg("cannot unset planning condition")
		return err
	}
	return nil
}

// SetAssembledExamsDirty upserts the generated-exams state: dirty=true when an
// input changed, dirty=false right after a (re)generation.
//
// reason and changedAt are stored, not dropped: plexams.gui renders both in the
// stale banner (Nav.svelte:1024,1027).
func (db *PG) SetAssembledExamsDirty(ctx context.Context, dirty bool, reason string, t time.Time) error {
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	changedAt := t

	err := db.q(ctx).SetAssembledExamsState(ctx, sqlc.SetAssembledExamsStateParams{
		SemesterID: db.semesterID,
		Dirty:      dirty,
		Reason:     reasonPtr,
		ChangedAt:  &changedAt,
	})
	if err != nil {
		log.Error().Err(err).Bool("dirty", dirty).Msg("cannot set generated-exams state")
		return err
	}
	return nil
}

// GetAssembledExamsState returns the generated-exams state. A missing row means
// nothing has been generated yet and is reported as not dirty -- the same answer
// the missing document gave.
func (db *PG) GetAssembledExamsState(ctx context.Context) (*model.AssembledExamsState, error) {
	row, err := db.q(ctx).GetAssembledExamsState(ctx, db.semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &model.AssembledExamsState{Dirty: false}, nil
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get generated-exams state")
		return nil, err
	}

	return &model.AssembledExamsState{
		Dirty:     row.Dirty,
		Reason:    row.Reason,
		ChangedAt: row.ChangedAt,
	}, nil
}
