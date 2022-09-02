package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var primussCmd = &cobra.Command{
	Use:   "primuss",
	Short: "primuss [subcommand]",
	Long: `Handle primuss data.
	fix-ancode from to --- fix ancode in primuss data (exam and studentregs)`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// plexams := initPlexamsConfig()
		switch args[0] {
		case "fix-ancode":
			if len(args) < 3 {
				fmt.Println("need from and to")
			}

		default:
			fmt.Println("primuss called with unkown sub command")
		}
	},
}

func init() {
	rootCmd.AddCommand(primussCmd)
}
