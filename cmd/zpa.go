package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	plx "github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	zpaCmd = &cobra.Command{
		Use:   "zpa",
		Short: "zpa to/from db",
		Long: `Fetch from zpa and post to zpa.
	teacher     --- fetch teacher
	exams       --- fetch exams
	invigs      --- fetch invigilator requirements
	students    --- fetch zpa infos of students with registrations
	studentregs --- post student registrations to zpa
	upload-plan --- upload the exam list`,
		ValidArgs: []string{"teacher", "exams", "invigs", "students", "studentregs", "upload-plan"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			// TODO: wenn schon in der DB vorhanden, Änderungen anzeigen
			case "teacher":
				_, err := plexams.ImportTeachersFromZPA(context.Background(), plx.NewConsoleReporter())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get teachers")
				}

			// TODO: wenn schon in der DB vorhanden, Änderungen anzeigen
			case "exams":
				_, err := plexams.ImportExamsFromZPA(context.Background(), plx.NewConsoleReporter())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get exams")
				}

			// TODO: wenn schon in der DB vorhanden, Änderungen anzeigen
			case "invigs":
				_, err := plexams.ImportInvigilatorRequirementsFromZPA(context.Background(), plx.NewConsoleReporter())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get invigilator requirements")
				}

			case "students":
				_, _, err := plexams.GetStudentsFromZPA(context.Background(), plx.NewConsoleReporter())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get students")
				}

			case "studentregs":

				if len(jsonOutputFile) == 0 {
					jsonOutputFile = "studentregs.json"
				}

				_, _, err := plexams.PostStudentRegsToZPA(context.Background(), jsonOutputFile, plx.NewConsoleReporter())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get student regs")
				}

			case "upload-plan":

				// TODO: immer gleich validieren
				if len(jsonOutputFile) == 0 {
					jsonOutputFile = "plan.json"
					if withRooms {
						jsonOutputFile = "planWithRooms.json"
					}
					if withInvigilators {
						jsonOutputFile = "planWithInvigilators.json"
					}
				}
				if withInvigilators {
					withRooms = true
				}
				upload := !dryrun
				if upload {
					upload = confirm("really upload to zpa?", 1)
				}

				if upload {
					fmt.Println("ok, uploading to zpa")
				}

				examsPosted, err := plexams.UploadPlan(context.Background(), withRooms, withInvigilators, upload, plx.NewConsoleReporter())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot upload plan")
				}
				// write json to file
				json, err := json.MarshalIndent(examsPosted, "", " ")
				if err != nil {
					log.Error().Err(err).Msg("cannot marshal exams into json")
				}
				err = os.WriteFile(jsonOutputFile, json, 0644)
				if err != nil {
					log.Error().Err(err).Msg("cannot write exams to file")
				} else {
					fmt.Printf(" saved copy to %s\n", jsonOutputFile)
				}

				// validate
				_, err = plexams.ValidateZPADateTimes(plx.NewConsoleReporter())
				if err != nil {
					log.Error().Err(err).Msg("error when validating")
				}

			default:
				fmt.Println("zpa called with unknown sub command")
			}
		},
	}
	jsonOutputFile   string
	withRooms        bool
	withInvigilators bool
	dryrun           bool
)

func init() {
	rootCmd.AddCommand(zpaCmd)
	zpaCmd.Flags().StringVarP(&jsonOutputFile, "out", "o", "", "output (json) file")
	zpaCmd.Flags().BoolVarP(&withRooms, "rooms", "r", false, "upload with planned rooms")
	zpaCmd.Flags().BoolVarP(&withInvigilators, "invigilators", "i", false, "upload with planned invigilators (implies rooms)")
	zpaCmd.Flags().BoolVarP(&withInvigilators, "dryrun", "d", false, "do not upload to zpa")
}
