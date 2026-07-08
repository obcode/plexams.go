// Package bootstrap wires up the plexams GraphQL server: it loads the lean bootstrap
// config (.plexams.yaml, credentials, db.uri), resolves/pins the active semester,
// constructs the *plexams.Plexams instance and starts the HTTP/GraphQL server.
//
// This replaces the former Cobra CLI: plexams is now a server-only tool driven entirely
// through the GraphQL/REST API (consumed by plexams.gui). The only command-line surface
// left is the three bootstrap flags below.
package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/go-homedir"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	dbURI    string
	semester string
	verbose  bool
)

// Serve parses the bootstrap flags, loads the config, constructs the Plexams instance
// and starts the GraphQL server. It blocks until the server is shut down.
func Serve() error {
	flag.StringVar(&dbURI, "db-uri", "", "override db.uri from config file")
	flag.StringVar(&semester, "semester", "", "override semester from config file")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.BoolVar(&verbose, "v", false, "verbose output (shorthand)")
	flag.Parse()

	setupLogging()

	if err := initConfig(); err != nil {
		return err
	}

	plexams := newPlexams()
	graph.StartServer(plexams, viper.GetString("server.port"))
	return nil
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	output := zerolog.ConsoleWriter{Out: os.Stdout}
	if verbose {
		output.FormatLevel = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		}
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Logger = zerolog.New(output).With().Caller().Timestamp().Logger()
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
		// Per-semester config lives in the database (collection semester_config_input),
		// edited through the GUI; there is no <semester>.yaml merge anymore.
	} else {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return fmt.Errorf("config '.plexams.yaml' not found (searched in: ., %s)", home)
		}
		return fmt.Errorf("cannot read config '.plexams.yaml': %w", err)
	}

	return nil
}

func newPlexams() *plexams.Plexams {
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
			resolved, database, ok, connErr := resolveStartSemester(dbURI)
			if connErr != nil {
				log.Fatal().Err(connErr).Msg("cannot connect to the database (check db.uri / network)")
			}
			if !ok {
				log.Fatal().Msg("database has no usable (compatible) workspace yet — " +
					"pin a semester with --semester <YYYY-SS> (e.g. --semester 2026-SS), " +
					"or create/restore one through the GUI (createSemester / semester-dump upload)")
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
// (last active, else newest compatible). connErr is non-nil when the database is
// unreachable; ok=false with connErr==nil means connected but no usable workspace.
func resolveStartSemester(dbURI string) (semester, database string, ok bool, connErr error) {
	client, err := db.NewDB(dbURI, "plexams", nil)
	if err != nil {
		return "", "", false, err
	}
	defer func() {
		if err := client.Client.Disconnect(context.Background()); err != nil {
			log.Debug().Err(err).Msg("cannot disconnect temporary client")
		}
	}()
	semester, database, ok = client.ResolveStartSemester(context.Background())
	return semester, database, ok, nil
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
