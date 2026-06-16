package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/go-homedir"
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

			return initConfig()
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
		if strings.TrimSpace(semester) == "" {
			return fmt.Errorf("missing setting 'semester' in .plexams.yaml")
		}
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

		err = semesterViper.ReadInConfig()
		if err != nil {
			var notFound viper.ConfigFileNotFoundError
			if errors.As(err, &notFound) {
				if p != "" {
					return fmt.Errorf("semester config '%s.yaml' not found (searched in: ., %s, %s)", semester, home, p)
				}
				return fmt.Errorf("semester config '%s.yaml' not found (searched in: ., %s)", semester, home)
			}
			return fmt.Errorf("cannot read semester config '%s.yaml': %w", semester, err)
		}

		err = viper.MergeConfigMap(semesterViper.AllSettings())
		if err != nil {
			return fmt.Errorf("cannot merge semester config: %w", err)
		}

		// Beobachte die per-Semester-Config (enthält z.B. invigilatorConstraints),
		// damit Änderungen ohne Neustart wirksam werden. viper liest die Datei vor
		// dem Callback selbst neu ein, wir mergen die frischen Werte ins globale
		// viper. Hinweis: Merge entfernt keine im File gelöschten Schlüssel – dafür
		// ist weiterhin ein Neustart nötig.
		semesterViper.OnConfigChange(func(e fsnotify.Event) {
			log.Info().Str("file", e.Name).Msg("semester config changed, reloading")
			if err := viper.MergeConfigMap(semesterViper.AllSettings()); err != nil {
				log.Error().Err(err).Msg("cannot re-merge semester config after change")
			}
		})
		semesterViper.WatchConfig()
	} else {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return fmt.Errorf("config '.plexams.yaml' not found (searched in: ., %s). Run 'plexams.go init'", home)
		}
		return fmt.Errorf("cannot read config '.plexams.yaml': %w", err)
	}

	return nil
}

func isConfigOptionalCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "init", "version":
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

	plexams, err := plexams.NewPlexams(
		strings.Replace(semester, "-", " ", 1),
		dbURI,
		viper.GetString("zpa.baseurl"),
		viper.GetString("zpa.username"),
		viper.GetString("zpa.password"),
		viper.GetStringSlice("zpa.fk07programs"),
		viper.GetStringSlice("zpa.oldprograms"),
	)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	plexams.PrintInfo()
	return plexams
}
