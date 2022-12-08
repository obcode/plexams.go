package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	planCmd = &cobra.Command{
		Use:   "plan",
		Short: "plan [subcommand]",
		Long: `Manipulate the plan.
	move-to ancode day slot    --- move [ancode] to [day number] [slot number]
	lock-examgroup groupcode   --- lock exam group to slot
	unlock-examgroup groupcode --- unlock / allow moving
	lock                       --- lock the whole plan`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "move-to":
				if len(args) < 4 {
					log.Fatal("need ancode, day and slot number")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				day, err := strconv.Atoi(args[2])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[2])
				}
				slot, err := strconv.Atoi(args[3])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[3])
				}
				success, err := plexams.AddExamToSlot(context.Background(), ancode, day, slot)
				if err != nil {
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d to (%d,%d)\n", ancode, day, slot)
				}

			case "lock-examgroup":
				if len(args) < 2 {
					log.Fatal("need exam group code")
				}
				examGroupCode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				planEntry, examGroup, err := plexams.LockExamGroup(context.Background(), examGroupCode)
				if err != nil {
					os.Exit(1)
				}
				if planEntry != nil {
					fmt.Printf("successfully locked exam group %d to slot (%d,%d)\n",
						planEntry.ExamGroupCode, planEntry.DayNumber, planEntry.SlotNumber)
				}
				if examGroup != nil {
					for _, exam := range examGroup.Exams {
						fmt.Printf("  - %d. %s, %s\n",
							exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module)
					}
				}

			case "unlock-examgroup":
				if len(args) < 2 {
					log.Fatal("need exam group code")
				}
				examGroupCode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				planEntry, examGroup, err := plexams.UnlockExamGroup(context.Background(), examGroupCode)
				if err != nil {
					os.Exit(1)
				}
				if planEntry != nil {
					fmt.Printf("successfully unlocked exam group %d to slot (%d,%d)\n",
						planEntry.ExamGroupCode, planEntry.DayNumber, planEntry.SlotNumber)
				}
				if examGroup != nil {
					for _, exam := range examGroup.Exams {
						fmt.Printf("  - %d. %s, %s\n",
							exam.Exam.Ancode, exam.Exam.ZpaExam.MainExamer, exam.Exam.ZpaExam.Module)
					}
				}

			case "lock":
				err := plexams.LockPlan(context.Background())
				if err != nil {
					log.Fatalf("error %v", err)
					os.Exit(1)
				}

			default:
				fmt.Println("plan called with unkown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(planCmd)
}
