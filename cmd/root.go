package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "plexams.go",
	Short: "Planning exams.",
	Long:  `Planing exams.`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// if *debug {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	output := zerolog.ConsoleWriter{Out: os.Stdout}
	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
	}
	log.Logger = zerolog.New(output).With().Caller().Timestamp().Logger()
	// }
}

func initConfig() {
	viper.SetConfigName("plexams")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
}

func initPlexams() *plexams.Plexams {
	plexams, err := plexams.NewPlexams(
		viper.GetString("semester"),
		viper.GetString("db.uri"),
		viper.GetString("zpa.baseurl"),
		viper.GetString("zpa.username"),
		viper.GetString("zpa.password"),
	)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	return plexams
}
