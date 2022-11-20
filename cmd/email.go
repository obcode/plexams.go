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
nta --- send emails to teachers about nta.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "nta":
				err := plexams.SendHandicapsMails(context.Background(), run)
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			default:
				fmt.Println("email called with unkown sub command")
			}
		},
	}
	run bool
)

func init() {
	rootCmd.AddCommand(emailCmd)
	emailCmd.Flags().BoolVarP(&run, "run", "r", false, "really send")
}
