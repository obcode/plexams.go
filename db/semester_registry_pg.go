package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// semesterConfigFormatVersion is the shape of model.SemesterConfigInput this
// binary reads and writes.
const semesterConfigFormatVersion = 1

// Semester returns the semester the client is pointed at, e.g. "2026-WS".
func (db *PG) Semester() string {
	return db.semesterID
}

// DBHost returns the host:port of the connection, with credentials and any
// path/query stripped, so it is safe to display.
//
// This is MongoHost renamed -- the one symbol of this migration that reached
// plexams.gui. The GraphQL field is now serverInfo.dbHost, renamed together
// with its resolver, the admin digest mail template and the GUI.
func (db *PG) DBHost() string {
	return hostOf(db.uri)
}

// hostOf strips scheme, credentials and path from a connection string.
func hostOf(uri string) string {
	s := uri
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if at := strings.LastIndex(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return s
}

// semesterMetaFromRow maps a registry row onto the package type the callers
// already use. Under Mongo this was a document in each database's semester_meta
// collection; here the registry row *is* the meta.
func semesterMetaFromRow(row sqlc.Semester) *SemesterMeta {
	return &SemesterMeta{
		SchemaVersion: row.SchemaVersion,
		ReadOnly:      row.ReadOnly,
		LastDumpAt:    row.LastDumpAt,
	}
}

func (db *PG) semesterMetaOf(ctx context.Context, semesterID string) (*SemesterMeta, error) {
	row, err := db.q(ctx).GetSemester(ctx, semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("semesterID", semesterID).Msg("cannot get semester meta")
		return nil, err
	}
	return semesterMetaFromRow(row), nil
}

// GetSemesterMeta returns the meta of the current semester (nil when it is not
// registered).
func (db *PG) GetSemesterMeta(ctx context.Context) (*SemesterMeta, error) {
	return db.semesterMetaOf(ctx, db.semesterID)
}

// EnsureSemester registers a semester if it is not registered yet, stamping the
// schema version of a fresh row. An already registered one keeps its version --
// only Migrate moves that.
//
// This is what the three SetMetaSemester* variants collapsed into once the label
// stopped being stored: they differed only in which semester they wrote, and the
// label they wrote is now derived from the id.
func (db *PG) EnsureSemester(ctx context.Context, semester string, version int) error {
	err := db.q(ctx).EnsureSemester(ctx, sqlc.EnsureSemesterParams{
		ID:            semester,
		SchemaVersion: version,
	})
	if err != nil {
		log.Error().Err(err).Str("semester", semester).Msg("cannot register semester")
	}
	return err
}

// EnsureMeta registers the current semester.
func (db *PG) EnsureMeta(ctx context.Context, version int) error {
	return db.EnsureSemester(ctx, db.semesterID, version)
}

// SwitchTo repoints the client at another semester and returns the logical
// semester external systems use ("2026-WS" -> "2026 WS").
//
// This is where "one database per semester" became "one column": it assigns
// semesterID instead of databaseName. It no longer reads anything -- the label
// used to be looked up in the registry, with an override for test clones on top.
func (db *PG) SwitchTo(ctx context.Context, semester string) string {
	db.semesterID = semester
	return semesterName(semester)
}

// SetSemesterReadOnly sets the read-only flag of the current semester.
func (db *PG) SetSemesterReadOnly(ctx context.Context, readOnly bool) error {
	err := db.q(ctx).SetSemesterReadOnly(ctx, sqlc.SetSemesterReadOnlyParams{
		ID:       db.semesterID,
		ReadOnly: readOnly,
	})
	if err != nil {
		log.Error().Err(err).Bool("readOnly", readOnly).Msg("cannot set read-only flag")
	}
	return err
}

// SetLastDumpAt records when the current semester was last dumped as a full
// semester ZIP.
func (db *PG) SetLastDumpAt(ctx context.Context, at time.Time) error {
	err := db.q(ctx).SetSemesterLastDumpAt(ctx, sqlc.SetSemesterLastDumpAtParams{
		ID:         db.semesterID,
		LastDumpAt: &at,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot set last dump time")
	}
	return err
}

// AllSemesterNames lists the registered semesters, newest first.
//
// One query where Mongo needed a ListDatabaseNames plus two probes per database,
// and there is no list of system databases to exclude any more -- a row in
// `semester` is a semester by definition, which is what the "is this database a
// semester?" filter was approximating.
func (db *PG) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	rows, err := db.q(ctx).ListSemesters(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot list semesters")
		return nil, err
	}

	semesters := make([]*model.Semester, 0, len(rows))
	for _, row := range rows {
		version := row.SchemaVersion
		semesters = append(semesters, &model.Semester{
			ID:            row.ID,
			Compatible:    row.HasConfig,
			ReadOnly:      row.ReadOnly,
			SchemaVersion: &version,
		})
	}

	return semesters, nil
}

// GetSemesterConfigInput returns the raw, editable per-semester config (the
// source of truth) or nil when none has been stored yet.
func (db *PG) GetSemesterConfigInput(ctx context.Context) (*model.SemesterConfigInput, error) {
	return db.semesterConfigInputOf(ctx, db.semesterID)
}

// GetSemesterConfigInputFor returns the raw config of another semester, or nil.
// Used to seed a new semester from a previous one and to guard createSemester
// against overwriting.
func (db *PG) GetSemesterConfigInputFor(ctx context.Context, semester string) (*model.SemesterConfigInput, error) {
	return db.semesterConfigInputOf(ctx, semester)
}

func (db *PG) semesterConfigInputOf(ctx context.Context, semesterID string) (*model.SemesterConfigInput, error) {
	row, err := db.q(ctx).GetSemesterConfigInput(ctx, semesterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("semesterID", semesterID).Msg("cannot get semester config input")
		return nil, err
	}

	if row.FormatVersion != semesterConfigFormatVersion {
		err := fmt.Errorf("semester config was written in format version %d, this binary reads %d",
			row.FormatVersion, semesterConfigFormatVersion)
		log.Error().Err(err).Str("semesterID", semesterID).Msg("cannot get semester config input")
		return nil, err
	}

	var input model.SemesterConfigInput
	if err := json.Unmarshal(row.Config, &input); err != nil {
		log.Error().Err(err).Str("semesterID", semesterID).Msg("cannot decode semester config input")
		return nil, err
	}
	return &input, nil
}

// SaveSemesterConfigInput replaces the stored raw per-semester config.
func (db *PG) SaveSemesterConfigInput(ctx context.Context, input *model.SemesterConfigInput) error {
	return db.SaveSemesterConfigInputFor(ctx, db.semesterID, input)
}

// SaveSemesterConfigInputFor writes the raw config of another semester (used when
// creating a new one).
//
// The semester has to be registered: the foreign key rejects a config for one
// that is not. Under Mongo the insert created the database as a side effect,
// which is how a typo in a name produced a second, empty database that then
// showed up in the switcher.
func (db *PG) SaveSemesterConfigInputFor(ctx context.Context, semester string, input *model.SemesterConfigInput) error {
	blob, err := json.Marshal(input)
	if err != nil {
		log.Error().Err(err).Str("semesterID", semester).Msg("cannot encode semester config input")
		return err
	}

	err = db.q(ctx).SetSemesterConfigInput(ctx, sqlc.SetSemesterConfigInputParams{
		SemesterID:    semester,
		Config:        blob,
		FormatVersion: semesterConfigFormatVersion,
	})
	if err != nil {
		log.Error().Err(err).Str("semesterID", semester).Msg("cannot save semester config input")
	}
	return err
}

// SemesterHasConfig reports whether a semester carries a config (i.e. is usable
// with this code).
func (db *PG) SemesterHasConfig(ctx context.Context, semester string) bool {
	config, err := db.semesterConfigInputOf(ctx, semester)
	return err == nil && config != nil
}

// ResolveStartSemester picks the semester to start with when none is pinned: the
// last active one if it still has a config, otherwise the newest compatible one.
// ok is false when nothing usable exists.
func (db *PG) ResolveStartSemester(ctx context.Context) (semester string, ok bool) {
	if active, _ := db.GetActiveSemester(ctx); active != nil && active.Semester != "" {
		if db.SemesterHasConfig(ctx, active.Semester) {
			return active.Semester, true
		}
	}

	sems, err := db.AllSemesterNames(ctx)
	if err == nil {
		for _, s := range sems {
			if s.Compatible {
				return s.ID, true
			}
		}
	}
	return "", false
}

// Migrate checks the current semester's data-shape version. It no longer
// migrates anything: the one released migration renamed MongoDB collections, and
// there are no pre-existing data shapes in PostgreSQL -- a semester is created at
// CurrentSchemaVersion.
//
// What it still does is the guard the plan asks for: refuse to run against data
// written by a newer binary. goose owns the structure of the database,
// semester.schema_version owns what the rows mean, and the two must not be
// merged.
func (db *PG) Migrate(ctx context.Context) error {
	meta, err := db.GetSemesterMeta(ctx)
	if err != nil {
		return err
	}
	if meta == nil {
		return nil
	}

	if meta.SchemaVersion > CurrentSchemaVersion {
		log.Warn().Int("data", meta.SchemaVersion).Int("code", CurrentSchemaVersion).
			Str("semester", db.semesterID).
			Msg("semester was written by a newer version of plexams, not migrating")
		return nil
	}
	if meta.SchemaVersion < CurrentSchemaVersion {
		if meta.ReadOnly {
			log.Warn().Int("from", meta.SchemaVersion).Int("to", CurrentSchemaVersion).
				Str("semester", db.semesterID).
				Msg("semester is read-only, skipping migrations — unprotect it to migrate")
			return nil
		}
		// Nothing to run: no PostgreSQL semester can predate the cut-over.
		// Stamping it keeps the version honest instead of leaving data that
		// claims to be older than everything in it.
		if err := db.q(ctx).SetSemesterSchemaVersion(ctx, sqlc.SetSemesterSchemaVersionParams{
			ID:            db.semesterID,
			SchemaVersion: CurrentSchemaVersion,
		}); err != nil {
			log.Error().Err(err).Msg("cannot stamp schema version")
			return err
		}
	}
	return nil
}
