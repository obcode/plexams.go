package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	validateCmd = &cobra.Command{
		Use:   "validate",
		Short: "validate [subcommand]",
		Long: `Manipulate the validate.
	all         --- guess what :-)
	conflicts   --- check conflicts for each student
	constraints --- check if constraints hold`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "all":
				for {

					err := plexams.ValidateConflicts()
					if err != nil {
						os.Exit(1)
					}
					err = plexams.ValidateConstraints()
					if err != nil {
						os.Exit(1)
					}
					if Sleep == 0 {
						break
					}
					time.Sleep(time.Duration(Sleep) * time.Second)
				}

			case "conflicts":
				for {

					err := plexams.ValidateConflicts()
					if err != nil {
						os.Exit(1)
					}
					if Sleep == 0 {
						break
					}
					time.Sleep(time.Duration(Sleep) * time.Second)
				}

			case "constraints":
				for {

					err := plexams.ValidateConstraints()
					if err != nil {
						os.Exit(1)
					}
					if Sleep == 0 {
						break
					}
					time.Sleep(time.Duration(Sleep) * time.Second)
				}

			default:
				fmt.Println("validate called with unkown sub command")
			}
		},
	}
	Sleep int
)

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().IntVarP(&Sleep, "sleep", "s", 0, "sleep [s] seconds and validate again")

}
