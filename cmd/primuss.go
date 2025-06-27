package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gookit/color"
	"github.com/spf13/cobra"
)

var primussCmd = &cobra.Command{
	Use:   "primuss",
	Short: "primuss [subcommand]",
	Long: `Handle primuss data.
	add-ancode zpa-ancode program primuss-ancode  --- add ancode to zpa-data
	fix-ancode program from to         			  --- fix ancode in primuss data (exam and studentregs)
	rm-studentreg program ancode mtknr 			  --- remove a student registration
	add-studentreg program ancode mtknr           --- add a student registration`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexamsConfig()
		switch args[0] {
		case "add-ancode":
			if len(args) < 4 {
				log.Fatal("need program and primuss ancode")
			}
			ancode, err := strconv.Atoi(args[1])
			if err != nil {
				log.Fatalf("cannot convert %s to int\n", args[2])
			}
			program := args[2]
			primussAncode, err := strconv.Atoi(args[3])
			if err != nil {
				log.Fatalf("cannot convert %s to int\n", args[2])
			}

			ctx := context.Background()
			zpaExam, err := plexams.GetZPAExam(ctx, ancode)
			if err != nil {
				log.Fatalf("cannot get zpa exam with ancode %d\n", ancode)
			}

			fmt.Printf("Found exam: %d. %s, %s (%v)\n", zpaExam.AnCode, zpaExam.Module, zpaExam.MainExamer, zpaExam.PrimussAncodes)

			if !confirm(fmt.Sprintf("add primuss ancode %s/%d to zpa exam?", program, primussAncode), 10) {
				os.Exit(0)
			}

			err = plexams.AddAncode(ctx, ancode, program, primussAncode)
			if err != nil {
				log.Fatalf("cannot add ancode")
			}

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

		case "rm-studentreg":
			if len(args) < 4 {
				log.Fatal("need program, ancode and mtknr")
			}
			program := args[1]
			ancode, err := strconv.Atoi(args[2])
			if err != nil {
				log.Fatalf("cannot convert %s to int\n", args[2])
			}
			mtknr := args[3]

			fmt.Printf("removing student reg %s from %s/%d\n", mtknr, program, ancode)
			ctx := context.Background()

			deleteCount, err := plexams.RemoveStudentReg(ctx, program, ancode, mtknr)
			if err != nil {
				log.Fatalf("cannot remove student reg: %v", err)
			}

			fmt.Printf("deleted %d document", deleteCount)

			color.Green.Println("\n>>> please re-run `plexams.go prepare studentregs`")

		case "add-studentreg":
			if len(args) < 4 {
				log.Fatal("need program, ancode and mtknr")
			}
			program := args[1]
			ancode, err := strconv.Atoi(args[2])
			if err != nil {
				log.Fatalf("cannot convert %s to int\n", args[2])
			}
			mtknr := args[3]

			fmt.Printf("adding student reg %s from %s/%d\n", mtknr, program, ancode)
			ctx := context.Background()

			err = plexams.AddStudentReg(ctx, program, ancode, mtknr)
			if err != nil {
				log.Fatalf("cannot add student reg: %v", err)
			}

			color.Green.Println("\n>>> please re-run `plexams.go prepare studentregs`")

		default:
			fmt.Println("primuss called with unknown sub command")
		}
	},
}

func init() {
	rootCmd.AddCommand(primussCmd)
}
