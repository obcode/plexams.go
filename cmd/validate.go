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
	db          --- data base entries
	rooms       --- check room constraints
	zpa         --- check if the plan on ZPA is the same here
	invigilator-reqs
	invigilator-slots

	-s <seconds> --- sleep <seconds> and validate again`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()

			validations := make([]func() error, 0)
			for _, arg := range args {
				switch arg {
				case "all":
					validations = append(validations, []func() error{
						func() error { return plexams.ValidateConflicts(OnlyPlannedByMe, Ancode) },
						plexams.ValidateConstraints,
						plexams.ValidateRoomsPerSlot,
						plexams.ValidateRoomsPerExam,
						plexams.ValidateRoomsTimeDistance,
					}...)

				case "conflicts":
					validations = append(validations,
						func() error { return plexams.ValidateConflicts(OnlyPlannedByMe, Ancode) },
					)

				case "constraints":
					validations = append(validations, plexams.ValidateConstraints)

				case "db":
					validations = append(validations, plexams.ValidateDB)

				case "rooms":
					validations = append(validations,
						[]func() error{
							plexams.ValidateRoomsPerSlot,
							plexams.ValidateRoomsNeedRequest,
							plexams.ValidateRoomsPerExam,
							plexams.ValidateRoomsTimeDistance,
						}...)

				case "zpa":
					err := plexams.ValidateZPADateTimes()
					if err != nil {
						log.Fatal(err)
					}
					if Rooms || Invigilators {
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

				case "invigilator-reqs":
					validations = append(validations, plexams.ValidateInvigilatorRequirements)

				case "invigilator-slots":
					validations = append(validations, plexams.ValidateInvigilatorSlots)

				default:
					fmt.Println("validate called with unknown sub command")
				}
			}
			validate(validations)
		},
	}
	Sleep           int
	Ancode          int
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
	validateCmd.Flags().IntVarP(&Ancode, "ancode", "a", 0, "show only constraints for given ancode")
	validateCmd.Flags().BoolVarP(&Clear, "clear", "c", false, "clear screen before output")
	validateCmd.Flags().BoolVarP(&Rooms, "rooms", "r", false, "validate zpa rooms")
	validateCmd.Flags().BoolVarP(&Invigilators, "invigilators", "i", false, "validate zpa invigilators")
	validateCmd.Flags().BoolVarP(&OnlyPlannedByMe, "onlyplannedbyme", "o", false, "check no conflicts if both exams are not planned by me")
}
