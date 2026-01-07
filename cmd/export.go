package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	exportCmd = &cobra.Command{
		Use:   "export",
		Short: "export [subcommand]",
		Long: `Generate various CSVs.
	planned-rooms - export rooms of planned exams.`,
		ValidArgs: []string{"planned-rooms"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "planned-rooms":
				if len(jsonfile) == 0 {
					jsonfile = "planned-rooms.json"
				}
				fmt.Printf("generating %s\n", jsonfile)
				err := plexams.ExportPlannedRooms(jsonfile)
				if err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("export called with unknown sub command")
			}
		},
	}
	jsonfile string
)

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.Flags().StringVarP(&jsonfile, "out", "o", "", "output (csv) file")
}
