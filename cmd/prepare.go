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
	nta             --- find NTAs for semester                     --- step 3
	exams-with-regs --- exams from connected-exams and studentregs --- step 4
	exam-groups     --- group of exams in the same slot            --- step 5
	partition       --- generate partition of groups               --- step 6`,
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

			case "exam-groups":
				err := plexams.PrepareExamsGroups()
				if err != nil {
					os.Exit(1)
				}

			case "partition":
				err := plexams.PartitionGroups()
				if err != nil {
					os.Exit(1)
				}

			case "nta":
				err := plexams.PrepareNta()
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
