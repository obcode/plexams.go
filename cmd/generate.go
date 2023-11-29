package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var (
	generateCmd = &cobra.Command{
		Use:   "generate [subcommand]",
		Short: "generate parts of the plan",
		Long: `Send parts of the plan.
plan --- generate the plan.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			plexams := initPlexamsConfig()
			switch args[0] {
			case "plan":
				err := plexams.GeneratePlan(context.Background()) // nolint
				if err != nil {
					log.Fatalf("got error: %v\n", err)
				}
			default:
				fmt.Println("generate called with unknown sub command")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(generateCmd)
}
