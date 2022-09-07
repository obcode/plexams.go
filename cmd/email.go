package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

var emailCmd = &cobra.Command{
	Use:   "email [subcommand]",
	Short: "send email",
	Long:  `Send emails.`,
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexamsConfig()
		err := plexams.SendTestMail()
		if err != nil {
			log.Fatalf("got error: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(emailCmd)
}
