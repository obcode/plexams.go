package plexams

import (
	"context"
	"fmt"

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

// loadSemesterMeta stamps the schema version (once, if the DB has a config) and
// loads the read-only flag of the current database into p.readOnly.
func (p *Plexams) loadSemesterMeta(ctx context.Context) {
	if p.dbClient == nil {
		return
	}
	if p.semesterConfig != nil {
		if err := p.dbClient.EnsureSchemaVersion(ctx, currentSchemaVersion); err != nil {
			log.Error().Err(err).Msg("cannot ensure schema version")
		}
	}
	p.readOnly = false
	if meta, err := p.dbClient.GetSemesterMeta(ctx); err != nil {
		log.Error().Err(err).Msg("cannot read semester meta")
	} else if meta != nil {
		p.readOnly = meta.ReadOnly
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

// SwitchSemester repoints the running instance to another semester at runtime.
//
// semester is the logical semester used against external systems (ZPA download/
// upload, stored doc labels), e.g. "2026 SS" — it stays the real semester even when
// the data lives in a differently named database. database is where the data lives;
// empty derives it from the semester. So a replay clone is switched with
// semester="2026 SS", database="2026-SS-Test", and a fresh re-import into an empty
// "2026-SS-Test" likewise keeps semester="2026 SS" so the ZPA import is correct.
//
// Single-user only: refused while an operation (validation/import/email/upload) is
// running; the GUI must refetch all data afterwards. The target may be empty (no
// config yet) — the config is then nil until created/imported.
func (p *Plexams) SwitchSemester(ctx context.Context, semester, database string) (*model.Semester, error) {
	if !p.WritesAllowed() {
		return nil, fmt.Errorf("cannot switch semester while an operation (validation/import/email/upload) is running")
	}

	p.semester = p.dbClient.SetSemester(semester, database)
	// force the ZPA client to be recreated with the new semester
	p.zpa.client = nil
	log.Info().Str("semester", p.semester).Msg("switched semester")

	p.loadSemesterConfig(ctx)
	if p.semesterConfig != nil {
		if err := p.dbClient.SaveSemesterConfig(ctx, p.semesterConfig); err != nil {
			log.Error().Err(err).Msg("cannot save semester config after switch")
		}
	} else {
		log.Warn().Str("semester", p.semester).Msg("switched to a semester/database without config (needs setup or import)")
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
