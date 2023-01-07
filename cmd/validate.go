package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

var (
	validateCmd = &cobra.Command{
		Use:   "validate",
		Short: "validate [subcommand] [-s <seconds>]",
		Long: `Validate the plan.
	all         --- guess what :-)
	conflicts   --- check conflicts for each student
	constraints --- check if constraints hold
	rooms       --- check room constraints
	
	-s <seconds> --- sleep <seconds> and validate again`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "all":
				validate([]func() error{
					plexams.ValidateConflicts,
					plexams.ValidateConstraints,
					plexams.ValidateRoomsPerSlot,
					plexams.ValidateRoomsPerExam,
				})

			case "conflicts":
				validate([]func() error{plexams.ValidateConflicts})

			case "constraints":
				validate([]func() error{plexams.ValidateConstraints})

			case "rooms":
				validate([]func() error{plexams.ValidateRoomsPerSlot, plexams.ValidateRoomsPerExam})

			default:
				fmt.Println("validate called with unkown sub command")
			}
		},
	}
	Sleep int
	Clear bool
)

func validate(funcs []func() error) {
	for {
		if Clear {
			c := exec.Command("clear")
			c.Stdout = os.Stdout
			c.Run() // nolint
		}
		for _, f := range funcs {
			err := f()
			if err != nil {
				os.Exit(1)
			}

		}
		if Sleep == 0 {
			return
		}
		time.Sleep(time.Duration(Sleep) * time.Second)
	}
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().IntVarP(&Sleep, "sleep", "s", 0, "sleep [s] seconds and validate again")
	validateCmd.Flags().BoolVarP(&Clear, "clear", "c", false, "clear screen before output")
}
