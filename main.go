//go:generate go run github.com/99designs/gqlgen generate --verbose
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/obcode/plexams.go/bootstrap"
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
		if info, ok := debug.ReadBuildInfo(); ok {
			var rev, vcsTime string
			var dirty bool
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					rev = s.Value
				case "vcs.time":
					vcsTime = s.Value
				case "vcs.modified":
					dirty = s.Value == "true"
				}
			}
			switch {
			case rev != "":
				// Lokaler/gowatch-Build: echten Commit statt der
				// irreführenden Pseudo-Version (v1.x, ignoriert v2/v3
				// mangels /vN im Modulpfad) zeigen.
				if len(rev) > 12 {
					rev = rev[:12]
				}
				version = "dev-" + rev
				if vcsTime != "" {
					version += " (" + vcsTime
					if dirty {
						version += ", dirty"
					}
					version += ")"
				}
			case info.Main.Version != "":
				version = info.Main.Version
			}
		}
	}
	viper.Set("Version", version)
	viper.Set("Commit", commit)
	viper.Set("Date", date)
	viper.Set("BuiltBy", builtBy)
	err := bootstrap.Serve()
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "Error:", err)
		if err != nil {
			panic(err)
		}
		os.Exit(-1)
	}
}
