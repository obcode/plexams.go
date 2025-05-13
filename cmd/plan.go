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
	pre-plan ancode day slot    --- move [ancode] to [day number] [slot number]
	move-to ancode day slot    --- move [ancode] to [day number] [slot number]
	change-room ancode oldroom newroom    --- change room for [ancode] from [oldroom] to [newroom]
	lock-exam ancode   --- lock exam to slot
	unlock-exam ancode --- unlock / allow moving
	remove-unlocked            --- remove all unlocked exam groups from the plan
	lock                       --- lock the whole plan`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "pre-plan":
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
				success, err := plexams.PreAddExamToSlot(context.Background(), ancode, day, slot)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d to (%d,%d)\n", ancode, day, slot)
				}
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
				success, err := plexams.AddExamToSlot(context.Background(), ancode, day, slot, force)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d to (%d,%d)\n", ancode, day, slot)
				}

			case "pre-move-to":
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
				success, err := plexams.PreAddExamToSlot(context.Background(), ancode, day, slot)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d to (%d,%d)\n", ancode, day, slot)
				}

			case "fixslotsindb":
				planEntries, err := plexams.PlanEntries(context.TODO())
				if err != nil {
					log.Fatal("cannot get plan entries")
				}

				for i, planEntry := range planEntries {
					fmt.Printf("%2d. fixing %v", i+1, planEntry)
					success, err := plexams.AddExamToSlot(context.Background(), planEntry.Ancode, planEntry.DayNumber, planEntry.SlotNumber, force)
					if err != nil {
						fmt.Printf("error: %v\n", err)
						os.Exit(1)
					}
					if success {
						fmt.Println(" ... success")
					}
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

			case "lock-exam":
				if len(args) < 2 {
					log.Fatal("need ancode")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				planEntry, exam, err := plexams.LockExam(context.Background(), ancode)
				if err != nil {
					os.Exit(1)
				}
				if planEntry != nil && exam != nil {
					fmt.Printf("successfully locked exam %d. %s, %s to slot (%d,%d)\n",
						exam.Ancode, exam.ZpaExam.MainExamer, exam.ZpaExam.Module, planEntry.DayNumber, planEntry.SlotNumber)
				}

			case "unlock-exam":
				if len(args) < 2 {
					log.Fatal("need ancode")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				planEntry, exam, err := plexams.UnlockExam(context.Background(), ancode)
				if err != nil {
					os.Exit(1)
				}
				if planEntry != nil && exam != nil {
					fmt.Printf("successfully unlocked exam %d. %s, %s from slot (%d,%d)\n",
						exam.Ancode, exam.ZpaExam.MainExamer, exam.ZpaExam.Module, planEntry.DayNumber, planEntry.SlotNumber)
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
	force bool
)

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.Flags().BoolVarP(&force, "force", "f", false, "force the operation")
}
