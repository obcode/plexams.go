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
	"io"
	"os"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/go-homedir"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph"
	"github.com/obcode/plexams.go/obs"
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

	// Error reporting before the logger is rebuilt, and both before newPlexams:
	// its four log.Fatal calls (unreachable database, no usable semester, ...)
	// are exactly the failures worth knowing about, and they only reach the
	// backend if the writer is already hanging in the logger when they fire.
	sentryWriter := initErrorReporting()

	// Only now can the logger honour log.format -- setupLogging runs first so that
	// a config error above is still logged like everything else.
	reconfigureLogging(sentryWriter)

	plexams := newPlexams()
	graph.StartServer(plexams, viper.GetString("server.port"))
	return nil
}

// setupLogging installs the human-readable console logger for the bootstrap
// window -- everything up to and including reading the config file. It cannot
// consult the config, because the config has not been read yet; that is
// reconfigureLogging's job.
func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Logger = zerolog.New(consoleWriter()).With().Caller().Timestamp().Logger()
}

// consoleWriter is the human-readable writer, shared by setupLogging and
// reconfigureLogging so the two cannot disagree about what "console" looks like.
func consoleWriter() zerolog.ConsoleWriter {
	output := zerolog.ConsoleWriter{Out: os.Stdout}
	if verbose {
		output.FormatLevel = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		}
	}
	return output
}

// reconfigureLogging rebuilds the global logger from the config, once it has
// been read. Only log.format is honoured; the level stays on the --verbose flag.
//
//	log:
//	  format: json     # or console (the default)
//
// Why JSON is worth having: ConsoleWriter is for humans and does not check
// whether it writes to a terminal (zerolog does not test isatty), so production
// container logs are ANSI-escaped prose. That is awkward to read back and, more
// importantly, can only be filtered by the SHAPE of a value. A log shipper that
// has to strip student data out of these lines can match `"mtknr":"..."` exactly
// in JSON, whereas on console text it would be reduced to guessing at digit runs
// -- and a rule like \b\d{7,10}\b eats epoch timestamps and durations too.
//
// sentryWriter, when non-nil, is hung alongside the local one: a log line at
// Error level or above then goes to the terminal AND to the error tracker.
// Note that this is independent of log.format -- zerolog hands both writers the
// same JSON, and the console writer is the one that reformats it.
func reconfigureLogging(sentryWriter zerolog.LevelWriter) {
	if !jsonLogging() && sentryWriter == nil {
		return // console only, exactly what setupLogging already installed
	}

	var out io.Writer = os.Stdout
	if !jsonLogging() {
		out = consoleWriter()
	}
	if sentryWriter != nil {
		out = zerolog.MultiLevelWriter(out, sentryWriter)
	}
	log.Logger = zerolog.New(out).With().Caller().Timestamp().Logger()
}

// initErrorReporting starts the error tracker and returns the log writer that
// feeds it, or nil when sentry.dsn is unset -- which is the default, and the
// whole of local development.
//
// A failure here is logged and swallowed on purpose: not being able to REPORT
// errors is not a reason to refuse to plan exams. It is also the one startup
// step whose own failure cannot be reported.
//
//	sentry:
//	  dsn: ""              # normally from $SENTRY_DSN, this is the rotated value
//	  environment: prod
//	  senduseremail: false # true sends the real address instead of a pseudonym
//	  ignoreerrors: []     # message patterns to drop; fill after a week of traffic
//	  debug: false
func initErrorReporting() zerolog.LevelWriter {
	writer, err := obs.Init(obs.Config{
		DSN:           viper.GetString("sentry.dsn"),
		Environment:   viper.GetString("sentry.environment"),
		Release:       viper.GetString("Version"),
		IgnoreErrors:  viper.GetStringSlice("sentry.ignoreerrors"),
		SendUserEmail: viper.GetBool("sentry.senduseremail"),
		// The KEK that already wraps the per-user Jira tokens, reused to key the
		// user pseudonym (obs/user.go). No key, no user -- see there for why
		// that is better than an unkeyed hash.
		UserKey: viper.GetString("secrets.key"),
		Debug:   viper.GetBool("sentry.debug"),
	})
	if err != nil {
		log.Error().Err(err).Msg("error reporting is disabled: cannot initialise it")
		return nil
	}
	if writer != nil {
		log.Info().Str("environment", viper.GetString("sentry.environment")).
			Msg("error reporting enabled")
	}
	return writer
}

// jsonLogging reports whether log.format selects machine-readable output.
func jsonLogging() bool {
	return strings.EqualFold(viper.GetString("log.format"), "json")
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
	// Map nested keys onto legal environment variable names: without this replacer
	// AutomaticEnv looks up db.uri as $DB.URI, which no shell can set -- so every
	// nested key was silently un-overridable and only the flat ones (semester,
	// verbose) ever worked. With it, db.uri reads $DB_URI.
	//
	// No SetEnvPrefix on purpose: a prefix would rename the flat keys too and break
	// the existing $SEMESTER / $VERBOSE lookups.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if semester == "" {
			semester = viper.GetString("semester")
		}
		// Watch the file so runtime re-reads pick up edits without a restart. The nightly
		// auto-sync re-reads scheduler.* on every fire, so recipient/source-toggle changes
		// take effect on the next run (scheduler.enabled/time are still bound at startup).
		viper.WatchConfig()
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
	// The version banner is a courtesy for a human at a terminal. In json mode it
	// would be the one line in the stream that no parser can read, so make it a
	// record like everything else.
	if jsonLogging() {
		log.Info().Str("version", viper.GetString("Version")).Msg("starting plexams.go")
	} else {
		fmt.Println(aurora.Sprintf(aurora.Cyan("Plexams.go version: %s\n"), viper.GetString("Version")))
	}

	if dbURI == "" {
		dbURI = viper.GetString("db.uri")
	}
	if semester == "" {
		semester = viper.GetString("semester")
	}
	// Bring the schema up before anything reads it. Against an empty database the
	// registry table does not exist yet, and the auto-selection below would report
	// "no usable semester" -- a wrong diagnosis for a database that is merely new.
	if err := migrateSchema(dbURI); err != nil {
		log.Fatal().Err(err).Msg("cannot migrate the database schema (check db.uri / network)")
	}

	// No semester pinned: auto-select the last active / newest compatible one.
	//
	// There is no db.database any more. It named the MongoDB database, which was
	// what let a clone be planned against under another name; a semester is a
	// semester_id now, and --semester says which.
	semester = strings.TrimSpace(semester)
	if semester == "" {
		resolved, ok, connErr := resolveStartSemester(dbURI)
		if connErr != nil {
			log.Fatal().Err(connErr).Msg("cannot connect to the database (check db.uri / network)")
		}
		if !ok {
			log.Fatal().Msg("database has no usable (compatible) semester yet — " +
				"pin one with --semester <YYYY-SS> (e.g. --semester 2026-SS), " +
				"or create one through the GUI (createSemester)")
		}
		semester = resolved
		log.Info().Str("semester", semester).Msg("auto-selected start semester")
	} else if !db.IsSemester(semester) {
		// Fail here rather than on the check constraint the first time anything
		// writes: a typo in the pin used to create a second, empty semester.
		log.Fatal().Str("semester", semester).
			Msg("not a semester (expected YYYY-SS or YYYY-WS, e.g. 2026-SS)")
	}

	plexams, err := plexams.NewPlexams(
		semester,
		dbURI,
		viper.GetString("zpa.baseurl"),
		viper.GetString("zpa.username"),
		viper.GetString("zpa.password"),
		viper.GetString("zpa.token"),
		viper.GetStringSlice("zpa.fk07programs"),
		viper.GetStringSlice("zpa.oldprograms"),
	)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create db client: %w", err))
	}

	// remember what we started with, so the next start can resume it
	plexams.RememberActiveSemester(context.Background())

	plexams.PrintInfo()
	return plexams
}

// migrateSchema applies the migrations the binary carries, on a connection of its
// own that is closed again right away.
//
// Its own connection because this runs before the Plexams client exists: the
// semester it would be constructed with is only known after the registry has been
// read, and reading the registry is what needs the schema.
func migrateSchema(dbURI string) error {
	ctx := context.Background()
	client, err := db.NewPG(ctx, dbURI, "")
	if err != nil {
		return err
	}
	defer client.Close()
	return client.MigrateSchema(ctx)
}

// resolveStartSemester opens a temporary DB connection to pick the start semester
// (last active, else newest compatible). connErr is non-nil when the database is
// unreachable; ok=false with connErr==nil means connected but nothing usable.
func resolveStartSemester(dbURI string) (semester string, ok bool, connErr error) {
	// Which semester is irrelevant here: the registry is one table, so this reads
	// across all of them. Under Mongo it had to connect to a named database first.
	client, err := db.NewPG(context.Background(), dbURI, "")
	if err != nil {
		return "", false, err
	}
	defer client.Close()
	semester, ok = client.ResolveStartSemester(context.Background())
	return semester, ok, nil
}
