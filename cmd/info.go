package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	infoCmd = &cobra.Command{
		Use:   "info [subcommand]",
		Short: "get info",
		Long: `Get info.
semester-config		    --- print config
exam [ancode]           --- print info about exam
samename                --- exams with same module name
goslots                 --- info about slots for GO/GN
request-rooms           --- which rooms to request
planned-room [roomname] --- get planned room for roomname
planned-ancodes		 	--- get planned ancodes
stats                   --- get statistics
student-regs ancode     --- get student-reqs for ancode
student name            --- get info for student.`,
		ValidArgs: []string{"semester-config", "exam", "samename", "goslots", "request-rooms", "planned-room", "planned-ancodes", "stats", "student-regs", "student"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			p := initPlexamsConfig()
			switch args[0] {
			case "semester-config":
				p.PrintSemesterConfig()
			case "exam":
				if len(args) < 2 {
					log.Fatal("need ancode")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				str, err := p.ExamInfo(ancode)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
				fmt.Println(str)
			case "samename":
				p.PrintSameName()
			case "goslots":
				err := plexams.PrintGOSlots(p.GetSemesterConfig().Slots, p.GetGoSlots())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "request-rooms":
				err := p.RequestRooms()
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "planned-room":
				if len(args) < 2 {
					log.Fatal("need roomname")
				}
				err := p.PlannedRoomInfo(args[1])
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "planned-ancodes":
				err := p.PlannedAncodes()
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "stats":
				err := p.PrintStatistics()
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "student-regs":
				if len(args) < 2 {
					log.Fatal("need ancode")
				}
				ancode, err := strconv.Atoi(args[1])
				if err != nil {
					log.Fatalf("cannot convert %s to int", args[1])
				}
				studentRegs, err := p.GetStudentRegsForAncode(ancode)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}

				formatted_data, err := json.MarshalIndent(studentRegs, "", " ")

				if err != nil {
					fmt.Println(err)
					return
				}

				f, err := os.Create(fmt.Sprintf("%d.json", ancode))

				if err != nil {
					log.Fatal(err)
				}

				defer f.Close() //nolint:errcheck

				_, err2 := f.Write(formatted_data)

				if err2 != nil {
					log.Fatal(err2)
				}

				fmt.Println("done")

			case "student":
				if len(args) < 2 {
					log.Fatal("need name")
				}
				err := p.PrintStudentInfo(args[1], long, zpa)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			default:
				fmt.Println("info called with unknown sub command")
			}
		},
	}
	long, zpa bool
)

func init() {
	rootCmd.AddCommand(infoCmd)
	infoCmd.Flags().BoolVarP(&long, "long", "l", false, "long info")
	infoCmd.Flags().BoolVarP(&zpa, "zpa", "z", false, "info from zpa")
}
