package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// SwitchSemester repoints the running instance to another semester/database (e.g.
// "2026 SS", "2026-SS" or a clone like "2026-SS-Test"). It reloads the per-semester
// config and refreshes the DB-derived globals. Single-user only: it refuses to
// switch while an operation (validation/import/email/upload) is running, and the
// GUI must refetch all data afterwards.
func (p *Plexams) SwitchSemester(ctx context.Context, name string) (*model.Semester, error) {
	if !p.WritesAllowed() {
		return nil, fmt.Errorf("cannot switch semester while an operation (validation/import/email/upload) is running")
	}

	// the target database must already carry a semester config
	input, err := p.dbClient.GetSemesterConfigInputForSemester(ctx, name)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("no semester config found for %q — create or clone it first", name)
	}

	p.semester = p.dbClient.SetSemester(name)
	log.Info().Str("semester", p.semester).Msg("switched semester")

	p.loadSemesterConfig(ctx)
	if p.semesterConfig != nil {
		if err := p.dbClient.SaveSemesterConfig(ctx, p.semesterConfig); err != nil {
			log.Error().Err(err).Msg("cannot save semester config after switch")
		}
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
