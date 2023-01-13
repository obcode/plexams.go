package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	invigilationCmd = &cobra.Command{
		Use:   "invigilation [subcommand]",
		Short: "Add an invigilation",
		Long: `Add an invigilation.
reserve [daynumber] [slotnumber] [invigilator ID] --- add reserve for slot (daynumber,slotnumber).`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "reserve":
				if len(args) < 4 {
					fmt.Println("need day number, slot nunbers, and the invigilators id")
					os.Exit(1)
				}
				day, err := strconv.Atoi(args[1])
				if err != nil {
					fmt.Printf("cannot use %s as day number", args[1])
					os.Exit(1)
				}
				slot, err := strconv.Atoi(args[2])
				if err != nil {
					fmt.Printf("cannot use %s as slot number", args[2])
					os.Exit(1)
				}
				invigilatorID, err := strconv.Atoi(args[3])
				if err != nil {
					fmt.Printf("cannot use %s as invigilators id", args[3])
					os.Exit(1)
				}

				err = plexams.AddReserveInvigilation(context.Background(), day, slot, invigilatorID)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			default:
				fmt.Println("invigilation called with ")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(invigilationCmd)
}
