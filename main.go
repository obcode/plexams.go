//go:generate go run github.com/99designs/gqlgen generate --verbose
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/obcode/plexams.go/cmd"
	"github.com/spf13/viper"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	loc, _ := time.LoadLocation("Europe/Berlin")
	time.Local = loc
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
	}
	viper.Set("Version", version)
	viper.Set("Commit", commit)
	viper.Set("Date", date)
	viper.Set("BuiltBy", builtBy)
	err := cmd.Execute()
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "Error:", err)
		if err != nil {
			panic(err)
		}
		os.Exit(-1)
	}
}
