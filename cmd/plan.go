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
	change-room ancode oldroom newroom    --- change room for [ancode] from [oldroom] to [newroom]
	lock-examgroup groupcode   --- lock exam group to slot
	unlock-examgroup groupcode --- unlock / allow moving
	remove-unlocked            --- remove all unlocked exam groups from the plan
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

			case "change-room":
				if len(args) < 4 {
					log.Fatal("need ancode, old room and new room names")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				oldRoom := args[2]
				newRoom := args[2]

				success, err := plexams.ChangeRoom(context.Background(), ancode, oldRoom, newRoom)
				if err != nil {
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d from %s to %s\n", ancode, oldRoom, newRoom)
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

			case "remove-unlocked":
				count, err := plexams.RemoveUnlockedExamGroupsFromPlan(context.Background())
				if err != nil {
					log.Fatalf("error %v", err)
					os.Exit(1)
				}
				fmt.Printf("successfully deleted %d unlocked exam groups from the plan\n", count)

			case "lock":
				err := plexams.LockPlan(context.Background())
				if err != nil {
					log.Fatalf("error %v", err)
					os.Exit(1)
				}

			default:
				fmt.Println("plan called with unknown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(planCmd)
}
