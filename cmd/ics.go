package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var (
	icsCmd = &cobra.Command{
		Use:   "ics",
		Short: "ics [subcommand]",
		Long: `Generate various icss.
	import-mucdai [filename] - import mucdai ics file`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "import-mucdai":
				if len(args) < 2 {
					log.Fatal("need filename")
				}
				err := plexams.ReadMucdaiICS(args[1])
				if err != nil {
					log.Fatalf("cannot read %s", args[1])
				}

			default:
				fmt.Println("pdf called with unknown sub command")
			}
		},
	}
	icsfile string
)

func init() {
	rootCmd.AddCommand(icsCmd)
	icsCmd.Flags().StringVarP(&icsfile, "out", "o", "", "output (ics) file")
}
