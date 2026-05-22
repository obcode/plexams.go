package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of plexams.go",
	Long:  `All software has versions. This is plexams.go'`,
	Run: func(cmd *cobra.Command, args []string) {
		if viper.GetString("Commit") == "none" {
			fmt.Printf("plexams.go version %s\n", viper.GetString("Version"))
		} else {
			fmt.Printf("plexams.go version %s, commit %s, build date %s, build by %s\n",
				viper.GetString("Version"),
				viper.GetString("Commit"),
				viper.GetString("Date"),
				viper.GetString("BuiltBy"),
			)
		}
	},
}
