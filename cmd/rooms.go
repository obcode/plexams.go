package cmd

import (
	"context"
	"fmt"

	plx "github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	roomsCmd = &cobra.Command{
		Use:   "rooms [subcommand]",
		Short: "get rooms",
		Long: `Get rooms.
anny		    --- fetch bookings from anny.eu.`,
		ValidArgs: []string{"anny"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			p := initPlexamsConfig()
			switch args[0] {
			case "anny":
				err := p.FetchFromAnny(context.Background(), plx.NewConsoleReporter())
				if err != nil {
					panic(err)
				}
			default:
				fmt.Println("info called with unknown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(roomsCmd)
}
