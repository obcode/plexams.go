package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	plx "github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	prepareCmd = &cobra.Command{
		Use:   "prepare",
		Short: "prepare [subcommand]",
		Long: `Prepare collections.
	connect-exam ancode program            --- connect an unconnected exam
	add-mucdai-exam program primuss-ancode --- add an external mucdai-exam exam
	add-mucdai-exams                       --- add all mucdai-exams
	generated-exams                        --- generate exams from connected-exams and primuss-data
	studentregs                            --- regs per exam & regs per student (needs connected-exams)
	rooms-for-exams                        --- rooms for exams
	self-invigilations                     --- set main examer as invigilator if possible
	invigilator-todos                      --- cache snapshot
	`,
		ValidArgs: []string{"connect-exam", "add-mucdai-exam", "generated-exams", "studentregs", "rooms-for-exams", "self-invigilations", "invigilator-todos"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {

			case "add-mucdai-exams":
				ctx := context.Background()
				mucdaiExams, err := plexams.MucdaiExams(ctx)
				if err != nil {
					panic(err)
				}

				for _, mucdaiExam := range mucdaiExams {
					if mucdaiExam.PlannedBy == "FK07" {
						continue
					}

					if !confirm(fmt.Sprintf("add external exam %s/%d. %s (%s)?",
						mucdaiExam.Program, mucdaiExam.PrimussAncode, mucdaiExam.Module, mucdaiExam.MainExamer), 10) {
						continue
					}

					zpaExam, err := plexams.AddMucDaiExamByProgram(ctx, mucdaiExam)
					if err != nil {
						os.Exit(1)
					}
					fmt.Printf("zpaAncode: %d\n", zpaExam.AnCode)
					prettyJSON, err := json.MarshalIndent(zpaExam, "", "    ")
					if err != nil {
						fmt.Println("Failed to generate json", err)
						return
					}

					// Print the pretty-printed JSON
					fmt.Println(string(prettyJSON))
				}

			case "add-mucdai-exam":
				if len(args) < 3 {
					log.Fatal("need program and primuss-ancode")
				}
				program := args[1]
				ancode, err := strconv.Atoi(args[2])
				if err != nil {
					fmt.Printf("cannot use %s as ancode", args[1])
					os.Exit(1)
				}

				ctx := context.Background()

				mucdaiExam, err := plexams.MucDaiExam(ctx, program, ancode)
				if err != nil {
					log.Fatal(err)
				}

				if !confirm(fmt.Sprintf("add external exam %s/%d. %s (%s)?",
					mucdaiExam.Program, mucdaiExam.PrimussAncode, mucdaiExam.Module, mucdaiExam.MainExamer), 10) {
					os.Exit(0)
				}

				zpaExam, err := plexams.AddMucDaiExamByProgram(ctx, mucdaiExam)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("zpaAncode: %d\n", zpaExam.AnCode)

				prettyJSON, err := json.MarshalIndent(zpaExam, "", "    ")
				if err != nil {
					fmt.Println("Failed to generate json", err)
					return
				}

				// Print the pretty-printed JSON
				fmt.Println(string(prettyJSON))

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

			case "generated-exams":
				err := plexams.PrepareGeneratedExams()
				if err != nil {
					os.Exit(1)
				}

			case "studentregs":
				err := plexams.PrepareStudentRegs()
				if err != nil {
					os.Exit(1)
				}

			case "rooms-for-exams":
				err := plexams.PrepareRoomForExams(context.Background(), plx.NewConsoleReporter())
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}

			case "self-invigilations":
				err := plexams.PrepareSelfInvigilation()
				if err != nil {
					os.Exit(1)
				}

			case "invigilator-todos":
				_, err := plexams.PrepareInvigilationTodos(context.Background())
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
