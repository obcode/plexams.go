package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
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
// lists — fk07programs, oldprograms (retired fk07), mucdaiprograms (DE/GS/ID) and
// miscprograms (default GN) — without overwriting any that already exist. Returns
// the number newly created.
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
		retired    bool
		external   bool // read externalExamsBase.<shortname> from config
		shortnames []string
	}{
		{"fk07", false, false, viper.GetStringSlice("zpa.fk07programs")},
		{"fk07", true, false, viper.GetStringSlice("zpa.oldprograms")},
		{"mucdai", false, true, viper.GetStringSlice("mucdaiprograms")},
		{"misc", false, true, miscPrograms},
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
			program := &model.StudyProgram{
				Shortname: shortname,
				Name:      "",
				Category:  set.category,
				Active:    !set.retired,
				Retired:   set.retired,
			}
			if set.external {
				if base := viper.GetInt(fmt.Sprintf("externalExamsBase.%s", shortname)); base > 0 {
					b := base
					program.ExternalExamsBase = &b
				}
			}
			if err := p.dbClient.UpsertStudyProgram(ctx, program); err != nil {
				return created, err
			}
			known[shortname] = struct{}{}
			created++
		}
	}
	return created, nil
}

// externalExamsBaseForProgram returns the base ancode for external (e.g. MUC.DAI)
// exams of a program from the StudyProgram master data.
func (p *Plexams) externalExamsBaseForProgram(ctx context.Context, program string) (int, bool) {
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read study programs for external exams base")
		return 0, false
	}
	for _, prog := range programs {
		if prog.Shortname == program && prog.ExternalExamsBase != nil {
			return *prog.ExternalExamsBase, true
		}
	}
	return 0, false
}

// fk07ProgramsFromStudyPrograms returns the current and old FK07 program shortnames
// from the StudyProgram master data (category fk07): current = not retired, old =
// retired. Returns (nil, nil) when no fk07 study programs are stored yet, so the
// caller can fall back to the config.
func (p *Plexams) fk07ProgramsFromStudyPrograms(ctx context.Context) (current, old []string, err error) {
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, prog := range programs {
		if prog.Category != "fk07" {
			continue
		}
		if prog.Retired {
			old = append(old, prog.Shortname)
		} else {
			current = append(current, prog.Shortname)
		}
	}
	if len(current) == 0 && len(old) == 0 {
		return nil, nil, nil
	}
	return current, old, nil
}
