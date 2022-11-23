package cmd

import (
	"fmt"
	"log"

	"github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	infoCmd = &cobra.Command{
		Use:   "info [subcommand]",
		Short: "get info",
		Long: `Gent info.
goslots --- info about slots for GO/GN.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			p := initPlexamsConfig()
			switch args[0] {
			case "goslots":
				err := plexams.PrintGOSlots(p.GetSemesterConfig().Slots, p.GetGoSlots())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			default:
				fmt.Println("email called with unkown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(infoCmd)
}
