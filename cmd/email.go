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
	emailCmd = &cobra.Command{
		Use:   "email [subcommand]",
		Short: "send email",
		Long: `Send emails.
primuss-data [all|<ancode>]   --- send emails to teachers about primuss data and nta
constraints 				  --- ask for constraints
prepared 					  --- announce exams to plan and constraints
draft 						  --- announce draft plan
published-exams 			  --- announce published exams
published-rooms 			  --- announce published rooms
invigilations 				  --- send email requesting invigilations constraints
published-invigilations       --- announce published invigilations
nta-with-room-alone 		  --- send emails to students with room alone before planning
nta-planned 				  --- send emails about rooms to all students with nta after planning
cover-pages [all|<teacherid>] --- send emails with externally generated cover pages
`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "primuss-data":
				if len(args) < 2 {
					log.Fatal("need ancode or all")
				}
				if args[1] == "all" {
					err := plexams.SendGeneratedExamMails(context.Background(), run)
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				} else {
					ancode, err := strconv.Atoi(args[1])
					if err != nil {
						fmt.Printf("cannot use %s as ancode", args[1])
						os.Exit(1)
					}
					err = plexams.SendGeneratedExamMail(context.Background(), ancode, run)
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				}
			case "constraints":
				err := plexams.SendEmailConstraints(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "prepared":
				err := plexams.SendEmailPrepared(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "published-exams":
				err := plexams.SendEmailPublishedExams(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "published-rooms":
				err := plexams.SendEmailPublishedRooms(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "published-invigilations":
				err := plexams.SendEmailPublishedInvigilations(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "invigilations":
				err := plexams.SendEmailInvigilations(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "draft":
				err := plexams.SendEmailDraft(run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "nta-with-room-alone":
				err := plexams.SendHandicapsMailsNTARoomAlone(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "nta-planned":
				err := plexams.SendHandicapsMailsNTAPlanned(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "cover-pages":
				if len(args) < 2 {
					log.Fatal("need teacher id or all")
				}
				if args[1] == "all" {
					err := plexams.SendCoverPagesMails(context.Background(), run)
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				} else {
					teacherID, err := strconv.Atoi(args[1])
					if err != nil {
						fmt.Printf("cannot use %s as teacher id", args[1])
						os.Exit(1)
					}
					err = plexams.SendCoverPageMail(context.Background(), teacherID, run)
					if err != nil {
						log.Fatalf("got error: %v\n", err)
					}
				}
			default:
				fmt.Println("email called with unknown sub command")
			}
		},
	}
	run bool
)

func init() {
	rootCmd.AddCommand(emailCmd)
	emailCmd.Flags().BoolVarP(&run, "run", "r", false, "really send")
}
