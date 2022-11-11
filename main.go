//go:generate go run github.com/99designs/gqlgen generate --verbose
package main

import (
	"github.com/obcode/plexams.go/cmd"
)

func main() {
	cmd.Execute()
}
