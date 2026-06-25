package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

// StudyPrograms returns all study programs (Studiengänge), global/cross-semester.
func (p *Plexams) StudyPrograms(ctx context.Context) ([]*model.StudyProgram, error) {
	return p.dbClient.StudyPrograms(ctx)
}

// UpsertStudyProgram creates or updates one study program (key: shortname).
func (p *Plexams) UpsertStudyProgram(ctx context.Context, program *model.StudyProgram) (*model.StudyProgram, error) {
	program.Shortname = strings.TrimSpace(program.Shortname)
	if program.Shortname == "" {
		return nil, fmt.Errorf("shortname (Kürzel) is required")
	}
	if program.Category == "" {
		program.Category = "misc"
	}
	if err := p.dbClient.UpsertStudyProgram(ctx, program); err != nil {
		return nil, err
	}
	return program, nil
}

// DeleteStudyProgram removes one study program by shortname.
func (p *Plexams) DeleteStudyProgram(ctx context.Context, shortname string) (bool, error) {
	return p.dbClient.DeleteStudyProgram(ctx, shortname)
}

// SeedStudyProgramsFromConfig creates study programs from the configured program
// lists — fk07programs, mucdaiprograms (DE/GS/ID) and miscprograms (default GN) —
// without overwriting any that already exist. Returns the number newly created.
func (p *Plexams) SeedStudyProgramsFromConfig(ctx context.Context) (int, error) {
	existing, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		return 0, err
	}
	known := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		known[e.Shortname] = struct{}{}
	}

	miscPrograms := viper.GetStringSlice("miscprograms")
	if len(miscPrograms) == 0 {
		miscPrograms = []string{"GN"}
	}

	seedSets := []struct {
		category   string
		shortnames []string
	}{
		{"fk07", viper.GetStringSlice("zpa.fk07programs")},
		{"mucdai", viper.GetStringSlice("mucdaiprograms")},
		{"misc", miscPrograms},
	}

	created := 0
	for _, set := range seedSets {
		for _, shortname := range set.shortnames {
			shortname = strings.TrimSpace(shortname)
			if shortname == "" {
				continue
			}
			if _, ok := known[shortname]; ok {
				continue
			}
			if err := p.dbClient.UpsertStudyProgram(ctx, &model.StudyProgram{
				Shortname: shortname,
				Name:      "",
				Category:  set.category,
				Active:    true,
			}); err != nil {
				return created, err
			}
			known[shortname] = struct{}{}
			created++
		}
	}
	return created, nil
}
