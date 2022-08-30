package cmd

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var zpaCmd = &cobra.Command{
	Use:   "zpa",
	Short: "zpa to/from db",
	Long: `Fetch from zpa and post to zpa.
	teacher --- fetch teacher
	exams --- fetch exams
	studentregs --- post student registrations to zpa`,
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

		case "studentregs":
			_, err := plexams.PostStudentRegsToZPA(context.Background())
			if err != nil {
				log.Fatal().Err(err).Msg("cannot get student regs")
			}
			// for i, studentReg := range studentRegs {
			// 	fmt.Printf("%5d. %+v\n", i+1, studentReg)
			// }

		default:
			fmt.Println("zpa called with unkown sub command")
		}
	},
}

func init() {
	rootCmd.AddCommand(zpaCmd)
}
