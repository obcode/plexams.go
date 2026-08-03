package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// DatabaseName returns the id of the workspace the client is pointed at.
//
// The name is kept although there is no database per semester any more: it is
// the workspace key, it appears in every generated file name
// (plexams/csv_export.go, dump.go), and renaming it would touch call sites that
// have nothing to do with storage.
func (db *PG) DatabaseName() string {
	return db.semesterID
}

// DBHost returns the host:port of the connection, with credentials and any
// path/query stripped, so it is safe to display.
//
// This is MongoHost renamed -- the one symbol of this migration that reaches
// plexams.gui, through serverInfo.mongoHost. The rename of the GraphQL field,
// its resolver, the admin digest mail template and the GUI belongs in the
// coordinated commit pair at cut-over, not here.
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
// collection; here the workspace row *is* the meta.
func semesterMetaFromRow(row sqlc.Semester) *SemesterMeta {
	return &SemesterMeta{
		SchemaVersion: row.SchemaVersion,
		ReadOnly:      row.ReadOnly,
		Semester:      row.Semester,
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

// GetSemesterMeta returns the meta of the current workspace (nil when there is
// no such workspace).
func (db *PG) GetSemesterMeta(ctx context.Context) (*SemesterMeta, error) {
	return db.semesterMetaOf(ctx, db.semesterID)
}

// EnsureMeta creates the workspace row if it is missing and stamps the schema
// version if it has none yet. It does NOT write the semester label: a derived or
// guessed label must not be persisted, only an explicit one (SetMetaSemester).
func (db *PG) EnsureMeta(ctx context.Context, version int) error {
	err := db.q(ctx).EnsureSemester(ctx, sqlc.EnsureSemesterParams{
		ID:            db.semesterID,
		SchemaVersion: version,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot stamp schema version")
	}
	return err
}

// SetMetaSemester force-writes the logical semester of the current workspace
// (authoritative: an explicit pin/override/createSemester).
func (db *PG) SetMetaSemester(ctx context.Context, semester string, version int) error {
	return db.setMetaSemesterIn(ctx, db.semesterID, semester, version)
}

// SetMetaSemesterForSemester force-writes the logical semester into the
// workspace derived from that semester's name (used when creating one).
func (db *PG) SetMetaSemesterForSemester(ctx context.Context, semester string, version int) error {
	return db.setMetaSemesterIn(ctx, databaseNameForSemester(semester), semester, version)
}

// SetMetaSemesterForDatabase force-writes the logical semester into a specific
// workspace (by exact id; used when creating a workspace).
func (db *PG) SetMetaSemesterForDatabase(ctx context.Context, database, semester string, version int) error {
	return db.setMetaSemesterIn(ctx, database, semester, version)
}

func (db *PG) setMetaSemesterIn(ctx context.Context, semesterID, semester string, version int) error {
	err := db.q(ctx).SetSemesterLabel(ctx, sqlc.SetSemesterLabelParams{
		ID:            semesterID,
		Semester:      semester,
		SchemaVersion: version,
	})
	if err != nil {
		log.Error().Err(err).Str("semesterID", semesterID).Msg("cannot set meta semester")
	}
	return err
}

// metaSemesterOf returns the stored logical semester of a workspace, or "".
func (db *PG) metaSemesterOf(ctx context.Context, semesterID string) string {
	if meta, _ := db.semesterMetaOf(ctx, semesterID); meta != nil {
		return meta.Semester
	}
	return ""
}

// SwitchTo repoints the client at the workspace named database. The logical
// semester is the override when given, else the workspace's stored one, else
// derived from its id. Returns the resolved logical semester.
//
// This is where "one database per semester" becomes "one column": it assigns
// semesterID instead of databaseName. Semantically identical, and still process-
// global -- de-globalising Plexams is a separate project.
func (db *PG) SwitchTo(ctx context.Context, database, override string) string {
	logical := strings.TrimSpace(override)
	if logical == "" {
		logical = db.metaSemesterOf(ctx, database)
	}
	if logical == "" {
		logical = semesterName(database)
	}
	db.semester = logical
	db.semesterID = database
	return logical
}

// SemesterForDatabase returns the logical semester of a workspace (its stored
// value, else derived from the id).
func (db *PG) SemesterForDatabase(ctx context.Context, database string) string {
	if logical := db.metaSemesterOf(ctx, database); logical != "" {
		return logical
	}
	return semesterName(database)
}

// SetSemesterReadOnly sets the read-only flag of the current workspace.
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

// SetLastDumpAt records when the current workspace was last dumped as a full
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

// AllSemesterNames lists the workspaces.
//
// One query where Mongo needed a ListDatabaseNames plus two probes per database,
// and there is no list of system databases to exclude any more -- a row in
// `semester` is a workspace by definition, which is what the isNotAWorkspace
// filter was approximating.
func (db *PG) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	rows, err := db.q(ctx).ListSemesters(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot list semesters")
		return nil, err
	}

	semesters := make([]*model.Semester, 0, len(rows))
	for _, row := range rows {
		version := row.SchemaVersion
		sem := &model.Semester{
			ID:            row.ID,
			Compatible:    row.HasConfig,
			ReadOnly:      row.ReadOnly,
			SchemaVersion: &version,
		}
		if row.Semester != "" {
			label := row.Semester
			sem.Semester = &label
		}
		semesters = append(semesters, sem)
	}

	// The ORDER BY already sorts by the stored label, but a workspace whose label
	// was never set has an empty one and would sort last. Mongo derived it from
	// the name in that case, so do the same here rather than in SQL.
	logicalOf := func(s *model.Semester) string {
		if s.Semester != nil {
			return *s.Semester
		}
		return semesterName(s.ID)
	}
	sort.SliceStable(semesters, func(i, j int) bool {
		li, lj := logicalOf(semesters[i]), logicalOf(semesters[j])
		if li != lj {
			return li > lj
		}
		return semesters[i].ID < semesters[j].ID
	})

	return semesters, nil
}

// GetSemesterConfigInput returns the raw, editable per-semester config (the
// source of truth) or nil when none has been stored yet.
func (db *PG) GetSemesterConfigInput(ctx context.Context) (*model.SemesterConfigInput, error) {
	return db.semesterConfigInputOf(ctx, db.semesterID)
}

// GetSemesterConfigInputForSemester returns the raw config of another semester,
// or nil. Used to seed a new semester from a previous one and to guard
// createSemester against overwriting.
func (db *PG) GetSemesterConfigInputForSemester(ctx context.Context, semester string) (*model.SemesterConfigInput, error) {
	return db.semesterConfigInputOf(ctx, databaseNameForSemester(semester))
}

// SemesterConfigInputForDatabase returns the raw config of a specific workspace
// (by exact id), or nil.
func (db *PG) SemesterConfigInputForDatabase(ctx context.Context, database string) (*model.SemesterConfigInput, error) {
	return db.semesterConfigInputOf(ctx, database)
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
	return db.SaveSemesterConfigInputToDatabase(ctx, db.semesterID, input)
}

// SaveSemesterConfigInputForSemester writes the raw config into the workspace
// derived from that semester's name (used when creating a new semester).
func (db *PG) SaveSemesterConfigInputForSemester(ctx context.Context, semester string, input *model.SemesterConfigInput) error {
	return db.SaveSemesterConfigInputToDatabase(ctx, databaseNameForSemester(semester), input)
}

// SaveSemesterConfigInputToDatabase writes the raw config into a specific
// workspace (by exact id).
//
// The workspace has to exist: the foreign key rejects a config for one that does
// not. Under Mongo the insert created the database as a side effect, which is
// how a typo in a workspace name produced a second, empty workspace that then
// showed up in the switcher.
func (db *PG) SaveSemesterConfigInputToDatabase(ctx context.Context, database string, input *model.SemesterConfigInput) error {
	blob, err := json.Marshal(input)
	if err != nil {
		log.Error().Err(err).Str("semesterID", database).Msg("cannot encode semester config input")
		return err
	}

	err = db.q(ctx).SetSemesterConfigInput(ctx, sqlc.SetSemesterConfigInputParams{
		SemesterID:    database,
		Config:        blob,
		FormatVersion: semesterConfigFormatVersion,
	})
	if err != nil {
		log.Error().Err(err).Str("semesterID", database).Msg("cannot save semester config input")
	}
	return err
}

// DatabaseHasConfig reports whether a workspace carries a semester config (i.e.
// is usable with this code).
func (db *PG) DatabaseHasConfig(ctx context.Context, databaseName string) bool {
	config, err := db.semesterConfigInputOf(ctx, databaseName)
	return err == nil && config != nil
}

// ResolveStartSemester picks the workspace to start with when none is pinned:
// the last active one if it still has a config, otherwise the newest compatible
// one. The returned database is empty when it should be derived from the
// semester. ok is false when nothing usable exists.
func (db *PG) ResolveStartSemester(ctx context.Context) (semester, database string, ok bool) {
	if active, _ := db.GetActiveSemester(ctx); active != nil && active.Database != "" {
		if db.DatabaseHasConfig(ctx, active.Database) {
			logical := db.metaSemesterOf(ctx, active.Database)
			if logical == "" {
				logical = active.Semester
			}
			if logical == "" {
				logical = semesterName(active.Database)
			}
			return logical, active.Database, true
		}
	}

	sems, err := db.AllSemesterNames(ctx)
	if err == nil {
		for _, s := range sems {
			if s.Compatible {
				return db.SemesterForDatabase(ctx, s.ID), s.ID, true
			}
		}
	}
	return "", "", false
}

// Migrate checks the workspace's data-shape version. It no longer migrates
// anything: the one released migration renamed MongoDB collections, and there
// are no pre-existing data shapes in PostgreSQL -- a workspace is created at
// CurrentSchemaVersion.
//
// What it still does is the guard the plan asks for: refuse to run against a
// workspace written by a newer binary. goose owns the structure of the database,
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
		log.Warn().Int("database", meta.SchemaVersion).Int("code", CurrentSchemaVersion).
			Str("database name", db.semesterID).
			Msg("database was written by a newer version of plexams, not migrating")
		return nil
	}
	if meta.SchemaVersion < CurrentSchemaVersion {
		if meta.ReadOnly {
			log.Warn().Int("from", meta.SchemaVersion).Int("to", CurrentSchemaVersion).
				Str("database", db.semesterID).
				Msg("database is read-only, skipping migrations — unprotect it to migrate")
			return nil
		}
		// Nothing to run: no PostgreSQL workspace can predate the cut-over.
		// Stamping it keeps the version honest instead of leaving a workspace
		// that claims to be older than everything in it.
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
