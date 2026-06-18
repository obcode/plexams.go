package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	plx "github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	validateCmd = &cobra.Command{
		Use:   "validate",
		Short: "validate [subcommand] [-s <seconds>]",
		Long: `Validate the plan.
	all                		--- guess what :-)
	conflicts          		--- check conflicts for each student
	constraints       	 	--- check if constraints hold
	preplanned-exahm-rooms 	--- validate exahm pre-planned rooms
	studentregs				--- check for students with registrations in diffenrent programs
	db                 		--- data base entries
	rooms              		--- check room constraints
	zpa                		--- check if the plan on ZPA is the same here
	invigilator-reqs.  		--- check if invigilator requirements are met (incl. shared constraints)
	invigilator-slots  		--- check if invigilator slots are okay
	invigilator-constraints	--- check the persisted plan against the shared invigplan constraints
`,
		ValidArgs: []string{"all", "conflicts", "constraints", "preplanned-exahm-rooms", "studentregs", "db", "rooms", "zpa", "invigilator-reqs", "invigilator-slots", "invigilator-constraints"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()

			validations := make([]func() error, 0)
			for _, arg := range args {
				switch arg {
				case "all":
					validations = append(validations, []func() error{
						plexams.ValidateDB,
						func() error { return plexams.ValidateConflicts(OnlyPlannedByMe, Ancode) },
						plexams.ValidateConstraints,
						func() error { _, err := plexams.ValidateRoomsPerSlot(plx.NewConsoleReporter()); return err },
						func() error { _, err := plexams.ValidateRoomsNeedRequest(plx.NewConsoleReporter()); return err },
						func() error { _, err := plexams.ValidateRoomsPerExam(plx.NewConsoleReporter()); return err },
						func() error { _, err := plexams.ValidateRoomsTimeDistance(plx.NewConsoleReporter()); return err },
					}...)

				case "conflicts":
					validations = append(validations,
						func() error { return plexams.ValidateConflicts(OnlyPlannedByMe, Ancode) },
					)

				case "constraints":
					validations = append(validations, plexams.ValidateConstraints)

				case "studentregs":
					validations = append(validations, plexams.ValidateStudentRegs)

				case "db":
					validations = append(validations, plexams.ValidateDB)

				case "preplanned-exahm-rooms":
					validations = append(validations, plexams.ValidatePrePlannedExahmRooms)

				case "rooms":
					validations = append(validations,
						[]func() error{
							func() error { _, err := plexams.ValidateRoomsPerSlot(plx.NewConsoleReporter()); return err },
							func() error { _, err := plexams.ValidateRoomsNeedRequest(plx.NewConsoleReporter()); return err },
							func() error { _, err := plexams.ValidateRoomsPerExam(plx.NewConsoleReporter()); return err },
							func() error { _, err := plexams.ValidateRoomsTimeDistance(plx.NewConsoleReporter()); return err },
						}...)

				case "zpa":
					_, err := plexams.ValidateZPADateTimes(plx.NewConsoleReporter())
					if err != nil {
						log.Fatal(err)
					}
					if Rooms || Invigilators {
						_, err := plexams.ValidateZPARooms(plx.NewConsoleReporter())
						if err != nil {
							log.Fatal(err)
						}
					}
					if Invigilators {
						_, err := plexams.ValidateZPAInvigilators(plx.NewConsoleReporter())
						if err != nil {
							log.Fatal(err)
						}
					}

				case "invigilator-reqs":
					validations = append(validations,
						func() error { _, err := plexams.ValidateInvigilatorRequirements(plx.NewConsoleReporter()); return err },
						func() error { _, err := plexams.ValidateInvigilationDups(plx.NewConsoleReporter()); return err },
						func() error {
							_, err := plexams.ValidateInvigilationsTimeDistance(plx.NewConsoleReporter())
							return err
						},
						func() error { _, err := plexams.ValidateInvigilationConstraints(plx.NewConsoleReporter()); return err },
					)

				case "invigilator-slots":
					validations = append(validations,
						func() error { _, err := plexams.ValidateInvigilatorSlots(plx.NewConsoleReporter()); return err })

				case "invigilator-constraints":
					validations = append(validations,
						func() error { _, err := plexams.ValidateInvigilationConstraints(plx.NewConsoleReporter()); return err })

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
		fmt.Printf("\n... sleeping %d seconds ...\n\n", Sleep)
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
