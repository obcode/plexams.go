package cmd

import (
	"fmt"
	"os"
	"strings"

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
	rootCmd  = &cobra.Command{
		Use:   "plexams.go",
		Short: "Planning exams.",
		Long:  `Planing exams.`,
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			plexams.PrintWorkflow()
			graph.StartServer(plexams, viper.GetString("server.port"))
		}}
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&dbURI, "db-uri", "",
		"override db.uri from config file")
	rootCmd.PersistentFlags().StringVar(&semester, "semester", "",
		"override semester from config file")

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
	home, err := homedir.Dir()
	if err != nil {
		er(err)
	}

	viper.AddConfigPath(home)
	viper.SetConfigName(".plexams")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if semester == "" {
			semester = viper.GetString("semester")
		}
		viper.AddConfigPath(fmt.Sprintf("%s/%s", viper.GetString("semester-path"), semester))
		viper.SetConfigName("plexams")
		err = viper.MergeInConfig()
		if err != nil {
			panic(fmt.Errorf("%s: should be %s.yml", err, "plexams"))
		}
	} else {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
}

func initPlexamsConfig() *plexams.Plexams {
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
	)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	plexams.PrintSemester()
	return plexams
}

func er(msg interface{}) {
	fmt.Println("Error:", msg)
	os.Exit(1)
}
