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
	pre-plan-exam ancode day slot                  --- move [ancode] to [day number] [slot number]
	pre-plan-room ancode roomname [mtknr/reserve]  --- plan [room name] for [ancode]
	move-to ancode day slot                        --- move [ancode] to [day number] [slot number]
	change-room ancode oldroom newroom             --- change room for [ancode] from [oldroom] to [newroom]
	lock-exam ancode                               --- lock exam to slot
	unlock-exam ancode                             --- unlock / allow moving
	lock                                           --- lock the whole plan`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "pre-plan-exam":
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

			case "pre-plan-room":
				if len(args) < 3 {
					log.Fatal("need ancode and room name and optional mtknr/reserve")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				roomName := args[2]
				var mtknr *string
				reserve := false
				if len(args) > 3 && args[3] != "" {
					if args[3] == "reserve" {
						reserve = true
					} else {
						mtknr = &args[3]
					}
				}
				success, err := plexams.PreAddRoomToExam(context.Background(), ancode, roomName, mtknr, reserve)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}
				if success {
					fmt.Printf("successfully moved exam %d to room %s\n", ancode, roomName)
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
