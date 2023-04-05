package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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
	studentregs --- post student registrations to zpa
	upload-plan --- upload the exam list`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "teacher":
				t := true
				teachers, err := plexams.GetTeachers(context.Background(), &t)
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get teachers")
				}
				for i, teacher := range teachers {
					fmt.Printf("%3d. %s\n", i+1, teacher.Fullname)
				}

			case "exams":
				t := true
				exams, err := plexams.GetZPAExams(context.Background(), &t)
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get teachers")
				}
				for _, exam := range exams {
					fmt.Printf("%3d. %s (%s)\n", exam.AnCode, exam.Module, exam.MainExamer)
				}

			case "invigs":
				invigs, err := plexams.GetSupervisorRequirements(context.Background())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get imvigilator requirements")
				}
				fmt.Printf("fetched %d invigilator requirements\n", len(invigs))

			case "studentregs":
				count, regsWithErrors, err := plexams.PostStudentRegsToZPA(context.Background())
				if err != nil {
					log.Fatal().Err(err).Msg("cannot get student regs")
				}
				fmt.Printf("%d successfully imported, %d errors\n", count, len(regsWithErrors))

			case "upload-plan":

				// TODO: immer gleich validieren
				// TODO: Wenn mehrere NTAs im gleichen Raum mit gleicher Zeit wird der Raum im ZPA nur einmal angezeigt => Räume zusammenfassen
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

				examsPosted, err := plexams.UploadPlan(context.Background(), withRooms, withInvigilators, upload)
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
