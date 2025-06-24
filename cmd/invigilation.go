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
		Use:   "invigilation [subcommand|roomname]",
		Short: "Add an invigilation",
		Long: `Add an invigilation.
reserve    [daynumber] [slotnumber] [invigilator ID] --- add reserve for slot (daynumber,slotnumber).
[roomname] [daynumber] [slotnumber] [invigilator ID] --- add invigilator for room in slot`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			ctx := context.Background()
			if len(args) < 4 {
				fmt.Println("need day number, slot numbers, and the invigilators id")
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

			room := args[0]
			if room != "reserve" {
				roomnames, err := plexams.PlannedRoomNamesInSlot(ctx, day, slot)
				if err != nil {
					fmt.Printf("error %s", err)
					os.Exit(1)
				}
				found := false
				for _, roomname := range roomnames {
					if room == roomname {
						found = true
					}
				}
				if !found {
					fmt.Printf("room %s not found in slot\n", room)
					os.Exit(1)
				}
			}

			invigilatorID, err := strconv.Atoi(args[3])
			if err != nil {
				fmt.Printf("cannot use %s as invigilators id", args[3])
				os.Exit(1)
			}

			oldInvigilator, err := plexams.GetInvigilatorInSlot(ctx, room, day, slot)
			if err != nil {
				os.Exit(1)
			}

			newInvigilator, err := plexams.GetInvigilator(ctx, invigilatorID)
			if err != nil {
				os.Exit(1)
			}
			if newInvigilator == nil {
				fmt.Printf("found no invigilator with id %d", invigilatorID)
				os.Exit(1)
			}

			if oldInvigilator != nil {
				if !confirm(fmt.Sprintf("Add \"%s\" and override existing invigilator \"%s\" in slot (%d,%d) for \"%s\"?",
					newInvigilator.Teacher.Shortname, oldInvigilator.Shortname, day, slot, room), 1) {
					os.Exit(0)
				}
			} else {
				if !confirm(fmt.Sprintf("Add \"%s\" for \"%s\" in slot (%d,%d)?",
					newInvigilator.Teacher.Shortname, room, day, slot), 1) {
					os.Exit(0)
				}
			}

			err = plexams.AddInvigilation(context.Background(), room, day, slot, invigilatorID)
			if err != nil {
				log.Fatalf("got error: %v\n", err)
			}

			fmt.Println("recalculating todos...")
			_, err = plexams.PrepareInvigilationTodos(context.Background())
			if err != nil {
				log.Fatalf("got error: %v\n", err)
			}
			fmt.Println("...done")

		},
	}
)

func init() {
	rootCmd.AddCommand(invigilationCmd)
}
