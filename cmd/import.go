package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "import [subcommand]",
	Long: `Restore data into the CURRENT semester database.
	semester-dump <file.zip> - restore a whole semester (only into a fresh/empty workspace).
	dataset <file.json>       - restore a single per-page dataset (use --name).
	dataset-csv <file.csv>    - restore a single entered dataset from human-readable CSV (use --name).

The target is the currently selected semester (see --semester/--db-uri). Use this to
re-upload a dump into a new test database.`,
	ValidArgs: []string{"semester-dump", "dataset"},
	Args:      cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexamsConfig()
		if plexams.IsReadOnly() {
			fmt.Println("semester is read-only, refusing to import")
			os.Exit(1)
		}

		switch args[0] {
		case "semester-dump":
			if len(args) < 2 {
				fmt.Println("import semester-dump requires a <file.zip>")
				os.Exit(1)
			}
			fmt.Printf("restoring semester from %s\n", args[1])
			result, err := plexams.ImportSemesterDump(args[1])
			if err != nil {
				fmt.Printf("restore failed: %v\n", err)
				os.Exit(1)
			}
			printRestoreResult(result.Restored, result.Total)

		case "dataset":
			if datasetName == "" {
				fmt.Println("import dataset requires --name")
				os.Exit(1)
			}
			if len(args) < 2 {
				fmt.Println("import dataset requires a <file.json>")
				os.Exit(1)
			}
			fmt.Printf("restoring dataset %q from %s\n", datasetName, args[1])
			result, err := plexams.ImportDataset(datasetName, args[1])
			if err != nil {
				fmt.Printf("restore failed: %v\n", err)
				os.Exit(1)
			}
			printRestoreResult(result.Restored, result.Total)

		case "dataset-csv":
			if datasetName == "" {
				fmt.Println("import dataset-csv requires --name")
				os.Exit(1)
			}
			if len(args) < 2 {
				fmt.Println("import dataset-csv requires a <file.csv>")
				os.Exit(1)
			}
			fmt.Printf("importing dataset %q from %s\n", datasetName, args[1])
			result, err := plexams.ImportDatasetCSVFile(datasetName, args[1])
			if err != nil {
				fmt.Printf("import failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("applied %d rows\n", result.Applied)
			for _, s := range result.Skipped {
				fmt.Printf("  skipped: %s\n", s)
			}

		default:
			fmt.Println("import called with unknown sub command")
		}
	},
}

func printRestoreResult(restored map[string]int, total int) {
	names := make([]string, 0, len(restored))
	for n := range restored {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("  %-30s %d\n", n, restored[n])
	}
	fmt.Printf("restored %d documents\n", total)
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().StringVar(&datasetName, "name", "", "dataset name (for 'import dataset')")
}
