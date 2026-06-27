package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// currentSchemaVersion is the data/schema version this code writes; bump it on a
// breaking change to a semester database's layout. minSupportedSchemaVersion is the
// oldest version this code can still work with.
const (
	currentSchemaVersion      = 1
	minSupportedSchemaVersion = 1
)

// IsReadOnly reports whether the current semester database is protected.
func (p *Plexams) IsReadOnly() bool {
	return p.readOnly
}

// loadSemesterMeta stamps the schema version and logical semester (when the DB has
// a config) and loads the read-only flag of the current database into p.readOnly.
func (p *Plexams) loadSemesterMeta(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	if p.semesterConfig != nil {
		if err := p.dbClient.EnsureMeta(ctx, currentSchemaVersion); err != nil {
			log.Error().Err(err).Msg("cannot ensure semester meta")
		}
	}
	p.readOnly = false
	if meta, err := p.dbClient.GetSemesterMeta(ctx); err != nil {
		log.Error().Err(err).Msg("cannot read semester meta")
	} else if meta != nil {
		p.readOnly = meta.ReadOnly
	}
}

// PersistSemester force-stores the current logical semester as the database's own
// (authoritative) semester. Use only for explicit values (a pin or override), never
// for a derived guess.
func (p *Plexams) PersistSemester(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	if err := p.dbClient.SetMetaSemester(ctx, p.semester, currentSchemaVersion); err != nil {
		log.Error().Err(err).Msg("cannot persist semester")
	}
}

// SetSemesterReadOnly protects/unprotects the current semester database.
func (p *Plexams) SetSemesterReadOnly(ctx context.Context, readOnly bool) (*model.Semester, error) {
	if err := p.dbClient.SetSemesterReadOnly(ctx, readOnly); err != nil {
		return nil, err
	}
	p.readOnly = readOnly
	log.Info().Str("semester", p.semester).Bool("readOnly", readOnly).Msg("set read-only")
	return p.GetSemester(ctx), nil
}

// SwitchSemester repoints the running instance to another database at runtime.
//
// name identifies the target database (an allSemesterNames label, e.g. "2026 SS" or
// a clone "2026 SS-Test"). The logical semester used against external systems (ZPA)
// is the database's own stored semester, so a clone keeps the real semester (e.g.
// "2026 SS") instead of its database name. semesterOverride is only needed for an
// empty database that has no stored semester yet (it is then remembered).
//
// Single-user only: refused while an operation (validation/import/email/upload) is
// running; the GUI must refetch all data afterwards. The target may be empty (no
// config yet) — the config is then nil until created/imported.
func (p *Plexams) SwitchSemester(ctx context.Context, name, semesterOverride string) (*model.Semester, error) {
	if !p.WritesAllowed() {
		return nil, fmt.Errorf("cannot switch semester while an operation (validation/import/email/upload) is running")
	}

	p.semester = p.dbClient.SwitchTo(ctx, name, semesterOverride)
	// force the ZPA client to be recreated with the new semester
	p.zpa.client = nil
	log.Info().Str("database", name).Str("semester", p.semester).Msg("switched semester")

	p.loadSemesterConfig(ctx)
	if p.semesterConfig != nil {
		if err := p.dbClient.SaveSemesterConfig(ctx, p.semesterConfig); err != nil {
			log.Error().Err(err).Msg("cannot save semester config after switch")
		}
	} else {
		log.Warn().Str("semester", p.semester).Msg("switched to a semester/database without config (needs setup or import)")
	}
	p.loadSemesterMeta(ctx)
	// an explicit override is authoritative for this database — remember it.
	if strings.TrimSpace(semesterOverride) != "" {
		p.PersistSemester(ctx)
	}
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
		p.planer = &Planer{Name: planer.Name, Email: planer.Email}
	}

	p.RememberActiveSemester(ctx)

	return p.GetSemester(ctx), nil
}

// RememberActiveSemester records the current semester/database as the last active
// one, so the next start can resume it (best-effort).
func (p *Plexams) RememberActiveSemester(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	if err := p.dbClient.SaveActiveSemester(ctx); err != nil {
		log.Error().Err(err).Msg("cannot remember active semester")
	}
}
