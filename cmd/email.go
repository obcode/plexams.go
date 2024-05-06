package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var (
	emailCmd = &cobra.Command{
		Use:   "email [subcommand]",
		Short: "send email",
		Long: `Send emails.
nta --- send emails to teachers about nta,
nta-with-room-alone --- send emails to students with room alone,
primuss-data --- send emails to teachers about primuss data and nta.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			// case "nta":
			// 	err := plexams.SendHandicapsMails(context.Background(), run)
			// 	if err != nil {
			// 		log.Fatalf("got error: %v\n", err)
			// 	}
			case "nta-with-room-alone":
				err := plexams.SendHandicapsMailsNTARoomAlone(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			case "primuss-data":
				err := plexams.SendGeneratedExamMails(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
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
