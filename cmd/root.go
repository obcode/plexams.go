package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/go-homedir"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dbURI    string
	semester string
	Verbose  bool
	rootCmd  = &cobra.Command{
		Use:   "plexams.go",
		Short: "Planning exams.",
		Long:  `Planing exams.`,
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			graph.StartServer(plexams, viper.GetString("server.port"))
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

			output := zerolog.ConsoleWriter{Out: os.Stdout}
			if Verbose {
				output.FormatLevel = func(i interface{}) string {
					return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
				}
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			} else {
				zerolog.SetGlobalLevel(zerolog.InfoLevel)
			}
			log.Logger = zerolog.New(output).With().Caller().Timestamp().Logger()

			if isConfigOptionalCommand(cmd) {
				return nil
			}

			if err := initConfig(); err != nil {
				return err
			}

			// audit-log mutating CLI invocations (same mutation_log collection as
			// the GraphQL middleware); read-only commands are skipped.
			logCLIInvocation(cmd, args)
			return nil
		},
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbURI, "db-uri", "",
		"override db.uri from config file")
	rootCmd.PersistentFlags().StringVar(&semester, "semester", "",
		"override semester from config file")
	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false,
		"verbose output")
}

func initConfig() error {
	home, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	viper.SetConfigName(".plexams")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath(home)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if semester == "" {
			semester = viper.GetString("semester")
		}
		// A pinned semester loads its optional per-semester YAML; without a pin the
		// semester is auto-selected from the database later (initPlexamsConfig).
		if strings.TrimSpace(semester) != "" {
			loadPerSemesterYAML(semester, home)
		}
	} else {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return fmt.Errorf("config '.plexams.yaml' not found (searched in: ., %s). Run 'plexams.go init'", home)
		}
		return fmt.Errorf("cannot read config '.plexams.yaml': %w", err)
	}

	return nil
}

// loadPerSemesterYAML merges the optional <semester>.yaml (from semester-path) into
// the global viper config and watches it for changes. The file is optional: the
// semester config is otherwise loaded from the database.
func loadPerSemesterYAML(semester, home string) {
	p := viper.GetString("semester-path")
	p = os.ExpandEnv(p)      // $HOME, $USER, ...
	p, _ = homedir.Expand(p) // ~ und ~user

	semesterViper := viper.New()
	semesterViper.SetConfigName(semester)
	semesterViper.SetConfigType("yaml")
	semesterViper.AddConfigPath(".")
	semesterViper.AddConfigPath(home)
	if p != "" {
		semesterViper.AddConfigPath(p)
	}

	if err := semesterViper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			log.Debug().Str("semester", semester).Msg("no per-semester YAML found, using config from the database")
		} else {
			log.Error().Err(err).Str("semester", semester).Msg("cannot read per-semester config")
		}
		return
	}

	if err := viper.MergeConfigMap(semesterViper.AllSettings()); err != nil {
		log.Error().Err(err).Msg("cannot merge semester config")
		return
	}

	// Watch the per-semester config so YAML changes take effect without a restart.
	// Note: merge does not remove keys deleted in the file – that still needs a restart.
	semesterViper.OnConfigChange(func(e fsnotify.Event) {
		log.Info().Str("file", e.Name).Msg("semester config changed, reloading")
		if err := viper.MergeConfigMap(semesterViper.AllSettings()); err != nil {
			log.Error().Err(err).Msg("cannot re-merge semester config after change")
		}
	})
	semesterViper.WatchConfig()
}

func isConfigOptionalCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "version":
			return true
		}
	}
	return false
}

func initPlexamsConfig() *plexams.Plexams {
	fmt.Println(aurora.Sprintf(aurora.Cyan("Plexams.go version: %s\n"), viper.GetString("Version")))

	if dbURI == "" {
		dbURI = viper.GetString("db.uri")
	}
	if semester == "" {
		semester = viper.GetString("semester")
	}
	// an explicitly pinned semester is authoritative for its database (persisted below)
	semesterPinned := strings.TrimSpace(semester) != ""
	dbOverride := viper.GetString("db.database")

	// No semester pinned: take the logical semester of a pinned database, else
	// auto-select the last active / newest compatible workspace from the database.
	if strings.TrimSpace(semester) == "" {
		if strings.TrimSpace(dbOverride) != "" {
			semester = logicalSemesterForDatabase(dbURI, dbOverride)
		} else {
			resolved, database, ok := resolveStartSemester(dbURI)
			if !ok {
				log.Fatal().Msg("no semester pinned and no usable (compatible) workspace found in the database")
			}
			semester = resolved
			if database != "" {
				viper.Set("db.database", database)
			}
			log.Info().Str("database", database).Str("semester", semester).Msg("auto-selected start workspace")
		}
	}

	plexams, err := plexams.NewPlexams(
		strings.Replace(semester, "-", " ", 1),
		dbURI,
		viper.GetString("zpa.baseurl"),
		viper.GetString("zpa.username"),
		viper.GetString("zpa.password"),
		viper.GetString("zpa.token"),
		viper.GetStringSlice("zpa.fk07programs"),
		viper.GetStringSlice("zpa.oldprograms"),
	)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	// a pinned semester is authoritative for its database (e.g. a replay clone with
	// semester=2026-SS + db.database=2026-SS-Test): remember the real semester there.
	if semesterPinned {
		plexams.PersistSemester(context.Background())
	}
	// remember what we started with, so the next start can resume it
	plexams.RememberActiveSemester(context.Background())

	plexams.PrintInfo()
	return plexams
}

// resolveStartSemester opens a temporary DB connection to pick the start semester
// (last active, else newest compatible). Returns ok=false when nothing usable.
func resolveStartSemester(dbURI string) (semester, database string, ok bool) {
	client, err := db.NewDB(dbURI, "plexams", nil)
	if err != nil {
		log.Error().Err(err).Msg("cannot connect to resolve start semester")
		return "", "", false
	}
	defer func() {
		if err := client.Client.Disconnect(context.Background()); err != nil {
			log.Debug().Err(err).Msg("cannot disconnect temporary client")
		}
	}()
	return client.ResolveStartSemester(context.Background())
}

// logicalSemesterForDatabase opens a temporary DB connection to read the logical
// semester stored in a specific database (else derives it from the name).
func logicalSemesterForDatabase(dbURI, database string) string {
	client, err := db.NewDB(dbURI, "plexams", nil)
	if err != nil {
		log.Error().Err(err).Msg("cannot connect to read database semester")
		return database
	}
	defer func() {
		if err := client.Client.Disconnect(context.Background()); err != nil {
			log.Debug().Err(err).Msg("cannot disconnect temporary client")
		}
	}()
	return client.SemesterForDatabase(context.Background(), database)
}
