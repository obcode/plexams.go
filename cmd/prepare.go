package cmd

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	prepareCmd = &cobra.Command{
		Use:   "prepare",
		Short: "prepare [subcommand]",
		Long: `Prepare collections.
	connected-exams --- prepare connected exams                    --- step 1
	connect-exam ancode program   --- connect an unconnected exam  --- step 1,5
	studentregs     --- regs per exam & regs per student           --- step 2
	exams-with-regs --- exams from connected-exams and studentregs --- step 3`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "connected-exams":
				err := plexams.PrepareConnectedExams()
				if err != nil {
					os.Exit(1)
				}

			case "connect-exam":
				if len(args) < 3 {
					log.Fatal("need ancode, program")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					fmt.Printf("cannot use %s as ancode", args[1])
					os.Exit(1)
				}
				program := args[2]

				err = plexams.ConnectExam(ancode, program)
				if err != nil {
					os.Exit(1)
				}

			case "studentregs":
				err := plexams.PrepareStudentRegs()
				if err != nil {
					os.Exit(1)
				}

			case "exams-with-regs":
				err := plexams.PrepareExamsWithRegs()
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
