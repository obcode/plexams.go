package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

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

	return p.GetSemester(ctx), nil
}
