package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	csvCmd = &cobra.Command{
		Use:   "csv",
		Short: "csv [subcommand]",
		Long: `Generate various CSVs.
	draft [program] - generate csv for program
	exahm - csv for EXaHM/SEB exams.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "draft":
				if len(args) < 2 {
					log.Fatal("need program")
				}
				program := args[1]
				if len(Outfile) == 0 {
					Outfile = fmt.Sprintf("VorläufigePrüfungsplanung_FK07_%s.csv", program)
				}
				fmt.Printf("generating %s\n", Outfile)
				err := plexams.CsvForProgram(program, Outfile)
				if err != nil {
					os.Exit(1)
				}

			case "exahm":
				if len(Outfile) == 0 {
					Outfile = "Prüfungsplanung_EXaHM_SEB_FK07.csv"
				}
				fmt.Printf("generating %s\n", Outfile)
				err := plexams.CsvForEXaHM(Outfile)
				if err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("pdf called with unknown sub command")
			}
		},
	}
	Csvfile string
)

func init() {
	rootCmd.AddCommand(csvCmd)
}
