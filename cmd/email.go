package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	plx "github.com/obcode/plexams.go/plexams"
	"github.com/spf13/cobra"
)

var (
	emailCmd = &cobra.Command{
		Use:   "email [subcommand]",
		Short: "send email",
		Long: `Send emails.
exahm                                         --- send email about EXaHM and SEB exams NEXT semester
primuss-data [all|<ancode>]                   --- send emails to teachers about primuss data and nta
primuss-data-unplanned <program> <ancode>     --- send emails to teachers about primuss data and nta
exam-planning-info                            --- send the consolidated exam-planning info email to the examers
draft                                         --- announce draft plan
published-exams                               --- announce published exams
published-rooms                               --- announce published rooms
invigilations                                 --- send email requesting invigilations constraints
invigilations-missing                         --- send email to profs with missing invigilator requirements in ZPA
published-invigilations                       --- announce published invigilations
new-nta                                       --- send emails to examers about new nta
nta-with-room-alone                           --- send emails to students with room alone before planning
nta-planned                                   --- send emails about rooms to all students with nta after planning
cover-pages [all|<teacherid>]                 --- send emails with externally generated cover pages
room-requests                                 --- send the request for the active building-management rooms
rooms-secretariat                             --- send the room occupancy (non-request rooms) to the secretariat for a ZPA check
kdp-exahm                                     --- send the EXaHM/SEB room overview (+CSV) to the KDP
lba-repeaters                                 --- send the LBA repeat-exam overview (dates/invigilations) to the LBA-BA
invigilations-secretariat                     --- tell the secretariat the invigilation plan is published (can be posted)
`,
		ValidArgs: []string{"primuss-data", "primuss-data-unplanned", "exam-planning-info", "draft", "published-exams", "published-rooms", "invigilations", "invigilations-missing", "published-invigilations", "new-nta", "nta-with-room-alone", "nta-planned", "cover-pages", "room-requests", "rooms-secretariat", "kdp-exahm", "lba-repeaters", "invigilations-secretariat"},
		Args:      cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			// On a dry run, bundle all mails into a single mail of .eml attachments
			// to the test address instead of sending each separately.
			if !run {
				plexams.BeginMailCollection()
				defer func() {
					if err := plexams.FlushMailCollection(plx.NewConsoleReporter()); err != nil {
						log.Printf("cannot flush bundled dry-run mails: %v\n", err)
					}
				}()
			}
			switch args[0] {
			case "exahm":
				err := plexams.SendEmailExaHM(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "primuss-data":
				if len(args) < 2 {
					log.Fatal("need ancode or all")
				}
				if args[1] == "all" {
					err := plexams.SendAssembledExamMails(context.Background(), false, run, plx.NewConsoleReporter())
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				} else {
					ancode, err := strconv.Atoi(args[1])
					if err != nil {
						fmt.Printf("cannot use %s as ancode", args[1])
						os.Exit(1)
					}
					err = plexams.SendAssembledExamMail(context.Background(), ancode, updated, run, plx.NewConsoleReporter())
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				}
			case "primuss-data-unplanned":
				if len(args) < 4 {
					log.Fatal("need program, ancode and email")
				}
				ancode, err := strconv.Atoi(args[2])
				if err != nil {
					fmt.Printf("cannot use %s as ancode", args[2])
					os.Exit(1)
				}
				program := args[1]
				email := args[3]
				err = plexams.SendUnplannedExamMail(context.Background(), program, ancode, email, run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "exam-planning-info":
				err := plexams.SendExamPlanningInfoMails(context.Background(), nil, run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "published-exams":
				err := plexams.SendEmailPublishedExams(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "published-rooms":
				err := plexams.SendEmailPublishedRooms(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "published-invigilations":
				err := plexams.SendEmailPublishedInvigilations(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "invigilations":
				err := plexams.SendEmailInvigilations(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "invigilations-missing":
				err := plexams.SendEmailInvigilationReqMissing(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "draft":
				err := plexams.SendEmailDraft(run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "new-nta":
				if len(args) < 2 {
					log.Fatal("need mtknr of new nta")
				}
				mtknr := args[1]
				err := plexams.SendMailNewNTA(context.Background(), mtknr, run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "nta-with-room-alone":
				if len(args) < 2 {
					log.Fatal("need mtknr of nta or \"all\"")
				}
				err := plexams.SendHandicapsMailsNTARoomAlone(context.Background(), args[1], run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "nta-planned":
				err := plexams.SendHandicapsMailsNTAPlanned(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "cover-pages":
				if len(args) < 2 {
					log.Fatal("need teacher id or all")
				}
				if args[1] == "all" {
					err := plexams.SendCoverPagesMails(context.Background(), run, plx.NewConsoleReporter())
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				} else {
					teacherID, err := strconv.Atoi(args[1])
					if err != nil {
						fmt.Printf("cannot use %s as teacher id", args[1])
						os.Exit(1)
					}
					err = plexams.SendCoverPageMail(context.Background(), teacherID, run, plx.NewConsoleReporter())
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				}
			case "room-requests":
				err := plexams.SendEmailRoomRequests(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "rooms-secretariat":
				err := plexams.SendEmailRoomsSecretariat(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "kdp-exahm":
				err := plexams.SendEmailKdpExahm(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "lba-repeaters":
				err := plexams.SendEmailLbaRepeaters(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "invigilations-secretariat":
				err := plexams.SendEmailInvigilationsSecretariat(context.Background(), run, plx.NewConsoleReporter())
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			default:
				fmt.Println("email called with unknown sub command")
			}
		},
	}
	run     bool
	updated bool
)

func init() {
	rootCmd.AddCommand(emailCmd)
	emailCmd.Flags().BoolVarP(&run, "run", "r", false, "really send")
	emailCmd.Flags().BoolVarP(&updated, "updated", "u", false, "updated data")
}
