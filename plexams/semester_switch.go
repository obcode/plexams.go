package plexams

import (
	"context"
	"errors"
	"fmt"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// currentSchemaVersion is the data/schema version this code writes; bump it on a
// breaking change to a semester database's layout. minSupportedSchemaVersion is the
// oldest version this code can still work with.
// They live next to the migrations that define them ([db/migrations.go]).
const (
	currentSchemaVersion      = db.CurrentSchemaVersion
	minSupportedSchemaVersion = db.MinSupportedSchemaVersion
)

// IsReadOnly reports whether the current semester is protected.
func (p *Plexams) IsReadOnly() bool {
	return p.readOnly
}

// loadSemesterMeta registers the current semester and stamps its schema version
// (when it has a config), and loads its read-only flag into p.readOnly.
func (p *Plexams) loadSemesterMeta(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	if p.semesterConfig != nil {
		if err := p.dbClient.EnsureMeta(ctx, currentSchemaVersion); err != nil {
			log.Error().Err(err).Msg("cannot ensure semester meta")
		}
	}
	// Migrate before indexing so the indexes land on the migrated collections. A
	// failed migration is logged and retried on the next start rather than being
	// fatal — the planner still needs to reach the GUI to diagnose it.
	if err := p.dbClient.Migrate(ctx); err != nil {
		log.Error().Err(err).Msg("cannot migrate database")
	}
	p.readOnly = false
	if meta, err := p.dbClient.GetSemesterMeta(ctx); err != nil {
		log.Error().Err(err).Msg("cannot read semester meta")
	} else if meta != nil {
		p.readOnly = meta.ReadOnly
	}
}

// SetSemesterReadOnly protects/unprotects the current semester.
func (p *Plexams) SetSemesterReadOnly(ctx context.Context, readOnly bool) (*model.Semester, error) {
	if err := p.dbClient.SetSemesterReadOnly(ctx, readOnly); err != nil {
		return nil, err
	}
	p.readOnly = readOnly
	log.Info().Str("semester", p.semester).Bool("readOnly", readOnly).Msg("set read-only")
	return p.GetSemester(ctx), nil
}

// SwitchSemester repoints the running instance to another semester at runtime.
//
// name is a semester id from allSemesterNames, e.g. "2026-SS"; the logical
// semester used against external systems (ZPA) is derived from it. There is no
// override any more: it existed for test clones, whose whole point was a database
// name that was not the semester.
//
// Single-user only: refused while an operation (validation/import/email/upload) is
// running; the GUI must refetch all data afterwards. The target may be empty (no
// config yet) — the config is then nil until created/imported.
func (p *Plexams) SwitchSemester(ctx context.Context, name string) (*model.Semester, error) {
	if !p.WritesAllowed() {
		return nil, fmt.Errorf("cannot switch semester while an operation (validation/import/email/upload) is running")
	}
	if !db.IsSemester(name) {
		return nil, fmt.Errorf("invalid semester %q (expected YYYY-SS or YYYY-WS)", name)
	}

	p.semester = p.dbClient.SwitchTo(ctx, name)
	// force the ZPA client to be recreated with the new semester
	p.zpa.client = nil
	log.Info().Str("semester", p.semester).Msg("switched semester")

	p.loadSemesterConfig(ctx)
	if p.semesterConfig == nil {
		log.Warn().Str("semester", p.semester).Msg("switched to a semester without config (needs setup or import)")
	}
	p.loadSemesterMeta(ctx)
	p.setRoomInfo()

	// keep the DB-derived globals consistent with the new semester's data
	if current, old, err := p.fk07ProgramsFromStudyPrograms(ctx); err != nil {
		log.Error().Err(err).Msg("cannot reload fk07 programs after switch")
	} else if len(current) > 0 || len(old) > 0 {
		p.zpa.fk07programs = current
		p.zpa.oldprograms = old
	}
	if planer, err := p.dbClient.GetPlaner(ctx); err != nil {
		log.Error().Err(err).Msg("cannot reload planer after switch")
	} else if planer != nil {
		p.applyPlaner(planer)
	}

	p.RememberActiveSemester(ctx)

	return p.GetSemester(ctx), nil
}

// RememberActiveSemester records the current semester as the last active one, so
// the next start can resume it (best-effort).
//
// A semester that is not registered yet is not a failure: --semester pins one on
// an empty database precisely so the server can start and createSemester can be
// called. There is simply nothing to resume until it exists.
func (p *Plexams) RememberActiveSemester(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	if err := p.dbClient.SaveActiveSemester(ctx); err != nil {
		if errors.Is(err, db.ErrSemesterNotRegistered) {
			log.Debug().Err(err).Msg("no active semester to remember yet")
			return
		}
		log.Error().Err(err).Msg("cannot remember active semester")
	}
}
