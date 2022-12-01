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
	planCmd = &cobra.Command{
		Use:   "plan",
		Short: "plan [subcommand]",
		Long: `Manipulate the plan.
	move-to --- move [ancode] to [day number] [slot number]`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "move-to":
				if len(args) < 4 {
					log.Fatal("need ancode, day and slot number")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				day, err := strconv.Atoi(args[2])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[2])
				}
				slot, err := strconv.Atoi(args[3])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[3])
				}
				success, err := plexams.AddExamToSlot(context.Background(), ancode, day, slot)
				if err != nil {
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d to (%d,%d)\n", ancode, day, slot)
				}

			default:
				fmt.Println("plan called with unkown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(planCmd)
}
