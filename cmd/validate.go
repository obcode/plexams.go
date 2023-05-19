package cmd

import (
	"fmt"
	"log"
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
	zpa         --- check if the plan on ZPA is the same here
	invigilations [reqs|slots]

	
	-s <seconds> --- sleep <seconds> and validate again`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "all":
				validate([]func() error{
					func() error { return plexams.ValidateConflicts(OnlyPlannedByMe) },
					plexams.ValidateConstraints,
					plexams.ValidateRoomsPerSlot,
					plexams.ValidateRoomsPerExam,
				})

			case "conflicts":
				validate([]func() error{func() error { return plexams.ValidateConflicts(OnlyPlannedByMe) }})

			case "constraints":
				validate([]func() error{plexams.ValidateConstraints})

			case "rooms":
				validate([]func() error{plexams.ValidateRoomsPerSlot, plexams.ValidateRoomsPerExam})

			case "zpa":
				fmt.Println("validating zpa dates and times")
				err := plexams.ValidateZPADateTimes()
				if err != nil {
					log.Fatal(err)
				}
				if Rooms || Invigilators {
					fmt.Println("validating zpa rooms")
					err := plexams.ValidateZPARooms()
					if err != nil {
						log.Fatal(err)
					}
				}
				if Invigilators {
					fmt.Println("validating zpa invigilators")
					err := plexams.ValidateZPAInvigilators()
					if err != nil {
						log.Fatal(err)
					}
				}

			case "invigilations":
				if len(args) == 1 {
					validate([]func() error{
						plexams.ValidateInvigilatorRequirements,
						plexams.ValidateInvigilatorSlots,
					})
				} else {
					switch args[1] {
					case "reqs":
						validate([]func() error{plexams.ValidateInvigilatorRequirements})

					case "slots":
						validate([]func() error{plexams.ValidateInvigilatorSlots})
					}
				}

			default:
				fmt.Println("validate called with unknown sub command")
			}
		},
	}
	Sleep           int
	Clear           bool
	Rooms           bool
	Invigilators    bool
	OnlyPlannedByMe bool
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
	validateCmd.Flags().BoolVarP(&Rooms, "room", "r", false, "validate zpa rooms")
	validateCmd.Flags().BoolVarP(&Invigilators, "invigilators", "i", false, "validate zpa invigilators")
	validateCmd.Flags().BoolVarP(&OnlyPlannedByMe, "onlyplannedbyme", "o", false, "check no conflicts if both exams are not planned by me")
}
