package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	prepareCmd = &cobra.Command{
		Use:   "prepare",
		Short: "prepare [subcommand]",
		Long: `Prepare collections.
	connected-exams                        --- prepare connected exams
	connect-exam ancode program            --- connect an unconnected exam
	add-mucdai-exam program primuss-ancode --- add an external add-mucdai-exam exam
	generated-exams                        --- generate exams from connected-exams and primuss-data
	studentregs                            --- regs per exam & regs per student (needs connected-exams)
	rooms-for-slots                        --- prepare rooms which are allowed to use
	rooms-for-exams                        --- rooms for exams
	self-invigilations                     --- set main examer as invigilator if possible
	invigilator-todos                      --- cache snapshot
	`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {

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

				// generate Ancode == prefix + PrimussAncode
				zpaAncode := viper.Get(fmt.Sprintf("externalExamsBase.%s", mucdaiExam.Program)).(int) + mucdaiExam.PrimussAncode
				fmt.Printf("zpaAncode: %d\n", zpaAncode)

				zpaExam, err := plexams.AddMucDaiExam(ctx, zpaAncode, mucdaiExam)
				if err != nil {
					os.Exit(1)
				}

				prettyJSON, err := json.MarshalIndent(zpaExam, "", "    ")
				if err != nil {
					fmt.Println("Failed to generate json", err)
					return
				}

				// Print the pretty-printed JSON
				fmt.Println(string(prettyJSON))

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

			case "rooms-for-slots":
				err := plexams.PrepareRoomsForSlots(approvedOnly)
				if err != nil {
					os.Exit(1)
				}

			case "rooms-for-exams":
				err := plexams.PrepareRoomForExams()
				if err != nil {
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
	approvedOnly bool
)

func init() {
	rootCmd.AddCommand(prepareCmd)
	prepareCmd.Flags().BoolVarP(&approvedOnly, "approvedOnly", "a", false, "use only already approved rooms for planning")
}
