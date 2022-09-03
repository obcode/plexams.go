package cmd

import (
	"context"
	"fmt"
	"log"
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

			// TODO:
			// 1. get primuss exam and ask
			exam, err := plexams.GetPrimussExam(context.Background(), program, from)
			if err != nil {
				log.Fatalf("error while trying to get exam: %v", err)
			}
			fmt.Printf("Found exam: %d. %s, %s\n", exam.AnCode, exam.Module, exam.MainExamer)
			// 1,5. check if new ancode is no exam
			noExam, err := plexams.GetPrimussExam(context.Background(), program, to)
			if err == nil {
				log.Fatalf("cannot change to new ancode, exam with ancode already exists: %d. %s, %s\n",
					noExam.AnCode, noExam.Module, noExam.MainExamer)
			}
			fmt.Println("Great! New ancode is free!")
			// 2. change primuss exam ancode
			// 3. change studentregs for exam
			// 4. log changes

		default:
			fmt.Println("primuss called with unkown sub command")
		}
	},
}

func init() {
	rootCmd.AddCommand(primussCmd)
}
