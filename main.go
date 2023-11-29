//go:generate go run github.com/99designs/gqlgen generate --verbose
package main

import (
	"time"

	"github.com/obcode/plexams.go/cmd"
)

func main() {
	loc, _ := time.LoadLocation("Europe/Berlin")
	time.Local = loc

	cmd.Execute()
}
