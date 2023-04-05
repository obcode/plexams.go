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
	partition       --- generate partition of groups               --- step 6
	
	Add exam after planning has started:

	connected-exam  --- prepare a connected exam for ancode
	exam-group      --- group of exams out of ancodes

	For planning rooms:

	exams-in-plan      --- split exam-groups into exams
	rooms-for-semester --- prepare rooms which are allowed to use
	rooms-for-exams    --- rooms for exams

	invigilate-self    --- set main examer as invigilator if possible
	`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "connected-exams":
				err := plexams.PrepareConnectedExams()
				if err != nil {
					os.Exit(1)
				}

			case "connected-exam":
				if len(args) < 2 {
					log.Fatal("need ancode")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					fmt.Printf("cannot use %s as ancode", args[1])
					os.Exit(1)
				}
				err = plexams.PrepareConnectedExam(ancode)
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
				err := plexams.PrepareExamGroups()
				if err != nil {
					os.Exit(1)
				}

			case "exam-group":
				if len(args) < 2 {
					log.Fatal("need ancode(s)")
				}
				ancodes := make([]int, 0, len(args)-1)
				for _, arg := range args[1:] {
					ancode, err := strconv.Atoi(arg)
					if err != nil {
						fmt.Printf("cannot use %s as ancode", args[1])
						os.Exit(1)
					}
					ancodes = append(ancodes, ancode)
				}

				err := plexams.PrepareExamGroup(ancodes)
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

			case "exams-in-plan":
				err := plexams.PreparePlannedExams()
				if err != nil {
					os.Exit(1)
				}

			case "rooms-for-semester":
				err := plexams.PrepareRoomsForSemester()
				if err != nil {
					os.Exit(1)
				}

			case "rooms-for-exams":
				err := plexams.PrepareRoomForExams()
				if err != nil {
					os.Exit(1)
				}

			case "invigilate-self":
				err := plexams.PrepareSelfInvigilation()
				if err != nil {
					os.Exit(1)
				}

			default:
				fmt.Println("prepare called with unknown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(prepareCmd)
}
