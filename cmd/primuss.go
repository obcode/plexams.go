package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var primussCmd = &cobra.Command{
	Use:   "primuss",
	Short: "primuss [subcommand]",
	Long: `Handle primuss data.
	fix-ancode program from to --- fix ancode in primuss data (exam and studentregs)`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexamsConfig()
		switch args[0] {
		case "fix-ancode":
			if len(args) < 4 {
				log.Fatal("need program, from and to")
			}
			program := args[1]
			from, err := strconv.Atoi(args[2])
			if err != nil {
				log.Fatalf("cannot convert %s to int\n", args[2])
			}
			to, err := strconv.Atoi(args[3])
			if err != nil {
				log.Fatalf("cannot convert %s to int\n", args[3])
			}

			fmt.Printf("changing ancode from %s/%d to %s/%d\n", program, from, program, to)
			ctx := context.Background()

			// 1. get primuss exam and ask
			exam, err := plexams.GetPrimussExam(ctx, program, from)
			if err != nil {
				log.Fatalf("error while trying to get exam: %v", err)
			}
			fmt.Printf("Found exam:\n    %s/%d. %s, %s\n", exam.Program, exam.AnCode, exam.Module, exam.MainExamer)

			studentRegs, err := plexams.GetStudentRegs(ctx, exam)
			if err != nil {
				log.Fatalf("error while trying to get student regs for exam: %v", err)
			}
			for i, studentReg := range studentRegs {
				fmt.Printf("    %2d. %s\n", i+1, studentReg.Name)
			}

			conflicts, err := plexams.GetConflicts(ctx, exam)
			if err != nil {
				log.Fatalf("error while trying to get conflicts for exam: %v", err)
			}
			for i, conflict := range conflicts.Conflicts {
				fmt.Printf("    %3d. conflict %d (%d students)\n", i+1, conflict.AnCode, conflict.NumberOfStuds)
			}

			// 2. check if new ancode is not yet used
			exists, err := plexams.PrimussExamExists(ctx, program, to)
			if err != nil {
				log.Fatalf("error while trying to check if exam with new ancode exists already: %v", err)
			}
			if exists {
				noExam, _ := plexams.GetPrimussExam(ctx, program, from)
				log.Fatalf("cannot change to new ancode, exam with ancode already exists: %d. %s, %s\n",
					noExam.AnCode, noExam.Module, noExam.MainExamer)
			}
			fmt.Println("Great! New ancode is not used!")
			if !confirm("change exam and student regs", 10) {
				os.Exit(0)
			}

			// 3. change primuss exam ancode
			newExam, err := plexams.ChangeAncode(ctx, program, from, to)
			if err != nil {
				log.Fatalf("error while trying to change the ancode from %s/%d to %s/%d\n",
					program, from, program, to)
			}
			fmt.Printf("Changed exam to:\n    %s/%d. %s, %s\n", newExam.Program, newExam.AnCode, newExam.Module, newExam.MainExamer)

			// 4. change studentregs and count for exam
			newStudentRegs, err := plexams.ChangeAncodeInStudentRegs(ctx, program, from, to)
			if err != nil {
				log.Fatalf("error while trying to change the ancode of all studentregs from %s/%d to %s/%d\n",
					program, from, program, to)
			}
			for i, studentReg := range newStudentRegs {
				fmt.Printf("    %2d. %s\n", i+1, studentReg.Name)
			}

			// 5. change conflicts
			newConflicts, err := plexams.ChangeAncodeInConflicts(ctx, program, from, to)
			if err != nil {
				log.Fatalf("error while trying to get conflicts for exam: %v", err)
			}
			for i, conflict := range newConflicts.Conflicts {
				fmt.Printf("    %3d. conflict %d (%d students)\n", i+1, conflict.AnCode, conflict.NumberOfStuds)
			}
			// 6. log changes
			err = plexams.Log(ctx, fmt.Sprintf("successfully changed primuss exam ancode from %s/%d to %s/%d",
				program, from, program, to))
			if err != nil {
				log.Fatalf("error while trying to log the change: %v", err)
			}

		default:
			fmt.Println("primuss called with unkown sub command")
		}
	},
}

func init() {
	rootCmd.AddCommand(primussCmd)
}
