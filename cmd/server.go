package cmd

import (
	"github.com/obcode/plexams.go/graph"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start GraphQL-Server.",
	Long:  `Start GraphQL-Server.`,
	Run: func(cmd *cobra.Command, args []string) {
		plexams := initPlexamsConfig()
		plexams.PrintWorkflow()
		graph.StartServer(plexams, viper.GetString("server.port"))
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
