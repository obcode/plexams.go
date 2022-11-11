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
	studentregs --- regs per exam & regs per student`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "studentregs":
				err := plexams.PrepareStudentRegs()
				if err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("pdf called with unkown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(prepareCmd)
}
