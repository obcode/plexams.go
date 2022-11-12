package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	prepareCmd = &cobra.Command{
		Use:   "prepare",
		Short: "prepare [subcommand]",
		Long: `Prepare collections.
	connected-exams --- prepare connected exams
	studentregs     --- regs per exam & regs per student`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "studentregs":
				err := plexams.PrepareStudentRegs()
				if err != nil {
					os.Exit(1)
				}

			case "connected-exams":
				err := plexams.PrepareConnectedExams()
				if err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("prepare called with unkown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(prepareCmd)
}
