package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

// releaseURL builds the GitHub release link for a build version, or "" for
// dev/unreleased builds. goreleaser sets the version without a leading "v"
// (e.g. "1.99.0"); a `go install ...@vX.Y.Z` build keeps it. Both map to the
// git tag "v1.99.0".
func releaseURL(version string) string {
	v := strings.TrimPrefix(version, "v")
	if v == "" || v[0] < '0' || v[0] > '9' {
		return ""
	}
	return "https://github.com/obcode/plexams.go/releases/tag/v" + v
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of plexams.go",
	Long:  `All software has versions. This is plexams.go'`,
	Run: func(cmd *cobra.Command, args []string) {
		version := viper.GetString("Version")
		if viper.GetString("Commit") == "none" {
			fmt.Printf("plexams.go version %s\n", version)
		} else {
			fmt.Printf("plexams.go version %s, commit %s, build date %s, build by %s\n",
				version,
				viper.GetString("Commit"),
				viper.GetString("Date"),
				viper.GetString("BuiltBy"),
			)
		}
		if url := releaseURL(version); url != "" {
			fmt.Printf("release: %s\n", url)
		}
	},
}
