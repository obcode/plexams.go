package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

var emailCmd = &cobra.Command{
	Use:   "email [subcommand]",
	Short: "send email",
	Long:  `Send emails.`,
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexamsConfig()
		err := plexams.SendHandicapsMailToMainExamer(context.Background(), 1)
		if err != nil {
			log.Fatalf("got error: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(emailCmd)
}
