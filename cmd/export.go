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
	planned-rooms - export rooms of planned exams.
	semester-dump - export the whole semester (all collections) as a ZIP.
	dataset       - export a single per-page dataset as JSON (use --name).`,
		ValidArgs: []string{"planned-rooms", "semester-dump", "dataset"},
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

			case "semester-dump":
				if len(jsonfile) == 0 {
					jsonfile = "semester-dump.zip"
				}
				fmt.Printf("writing %s\n", jsonfile)
				if err := plexams.ExportSemesterDump(jsonfile); err != nil {
					os.Exit(1)
				}

			case "dataset":
				if datasetName == "" {
					fmt.Println("export dataset requires --name")
					os.Exit(1)
				}
				if len(jsonfile) == 0 {
					jsonfile = fmt.Sprintf("%s.json", datasetName)
				}
				fmt.Printf("writing %s\n", jsonfile)
				if err := plexams.ExportDataset(datasetName, jsonfile); err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("export called with unknown sub command")
			}
		},
	}
	jsonfile    string
	datasetName string
)

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.Flags().StringVarP(&jsonfile, "out", "o", "", "output file")
	exportCmd.Flags().StringVar(&datasetName, "name", "", "dataset name (for 'export dataset')")
}
