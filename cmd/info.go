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
goslots       --- info about slots for GO/GN
request-rooms --- which rooms to request
stats         --- get statistics
student-regs ancode --- get student-reqs for ancode.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			p := initPlexamsConfig()
			switch args[0] {
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

				defer f.Close()

				_, err2 := f.Write(formatted_data)

				if err2 != nil {
					log.Fatal(err2)
				}

				fmt.Println("done")
			default:
				fmt.Println("info called with unknown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(infoCmd)
}
