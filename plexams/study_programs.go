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

// jointFacultyConfig is one joint Studienfakultät (e.g. MUC.DAI / MUC.HEALTH) and
// its study programs, as read from the `jointfaculties` config list.
type jointFacultyConfig struct {
	Name     string   `mapstructure:"name"`
	Programs []string `mapstructure:"programs"`
}

// jointFacultyConfigsFromViper reads the joint Studienfakultäten (each with its
// program list) from the `jointfaculties` config block. Falls back to the legacy
// flat `mucdaiprograms` list (seeded as MUC.DAI) when no `jointfaculties` block is
// present, so old configs keep working.
func jointFacultyConfigsFromViper() []jointFacultyConfig {
	var jfs []jointFacultyConfig
	if err := viper.UnmarshalKey("jointfaculties", &jfs); err != nil {
		log.Error().Err(err).Msg("cannot read jointfaculties config")
	}
	if len(jfs) == 0 {
		if legacy := viper.GetStringSlice("mucdaiprograms"); len(legacy) > 0 {
			jfs = []jointFacultyConfig{{Name: "MUC.DAI", Programs: legacy}}
		}
	}
	return jfs
}

// SeedStudyProgramsFromConfig creates study programs from the configured program
// lists — fk07programs, oldprograms (retired fk07), the joint Studienfakultäten
// (jointfaculties, e.g. MUC.DAI DE/GS/ID + MUC.HEALTH) and miscprograms (default
// GN) — without overwriting any that already exist. Returns the number newly created.
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

	type seedSet struct {
		category     string
		retired      bool
		external     bool   // read externalExamsBase.<shortname> from config
		jointFaculty string // set for category "joint"
		shortnames   []string
	}
	seedSets := []seedSet{
		{"fk07", false, false, "", viper.GetStringSlice("zpa.fk07programs")},
		{"fk07", true, false, "", viper.GetStringSlice("zpa.oldprograms")},
		{"misc", false, true, "", miscPrograms},
	}
	for _, jf := range jointFacultyConfigsFromViper() {
		seedSets = append(seedSets, seedSet{"joint", false, true, jf.Name, jf.Programs})
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
				ZpaCode:   defaultZpaCode(shortname),
			}
			if set.jointFaculty != "" {
				jf := set.jointFaculty
				program.JointFaculty = &jf
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

// defaultZpaCode derives the external ZPA code for a (possibly degree-suffixed)
// program shortname when seeding: an explicit `zpacodes.<shortname>` config entry
// wins, otherwise a trailing "-B"/"-M" degree suffix is stripped (DC-B → DC).
// Returns "" when the code equals the shortname (identity — no mapping needed).
func defaultZpaCode(shortname string) string {
	if code := strings.TrimSpace(viper.GetString("zpacodes." + shortname)); code != "" {
		if code == shortname {
			return ""
		}
		return code
	}
	for _, suffix := range []string{"-B", "-M"} {
		if strings.HasSuffix(shortname, suffix) {
			return strings.TrimSuffix(shortname, suffix)
		}
	}
	return ""
}

// zpaCodeForProgram returns the external ZPA code (the 2-letter code ZPA uses) for
// an internal, possibly degree-suffixed program shortname. It is the reverse of the
// inbound ZPA-group→program mapping and is used when posting student registrations
// back to ZPA. Falls back to the shortname itself when no ZpaCode is stored — the
// identity case before the suffix rename and once ZPA adopts unique codes.
func (p *Plexams) zpaCodeForProgram(ctx context.Context, shortname string) string {
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read study programs for zpa code mapping")
		return shortname
	}
	for _, prog := range programs {
		if prog.Shortname == shortname && prog.ZpaCode != "" {
			return prog.ZpaCode
		}
	}
	return shortname
}

// migrateMucdaiToJoint upgrades legacy study programs (category "mucdai") to the
// generalized joint-program model: category "joint" + jointFaculty "MUC.DAI".
// Idempotent (a no-op once migrated). Global master data (~3 docs), run at startup.
func (p *Plexams) migrateMucdaiToJoint(ctx context.Context) {
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read study programs for mucdai→joint migration")
		return
	}
	mucDai := "MUC.DAI"
	for _, prog := range programs {
		if prog.Category != "mucdai" {
			continue
		}
		prog.Category = "joint"
		if prog.JointFaculty == nil {
			prog.JointFaculty = &mucDai
		}
		if err := p.dbClient.UpsertStudyProgram(ctx, prog); err != nil {
			log.Error().Err(err).Str("shortname", prog.Shortname).Msg("cannot migrate mucdai program to joint")
		}
	}
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
