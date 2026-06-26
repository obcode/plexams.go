package cmd

import (
	"context"
	"strconv"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// cliReadOnlyCommands are top-level commands that never change data, so their
// invocations are not audit-logged.
var cliReadOnlyCommands = map[string]bool{
	"info":       true,
	"validate":   true,
	"export":     true,
	"csv":        true,
	"pdf":        true,
	"ics":        true,
	"version":    true,
	"server":     true,
	"help":       true,
	"completion": true,
	"plexams.go": true, // the bare root (starts the server)
}

// logCLIInvocation records a mutating CLI command in the per-semester mutation_log
// collection, so CLI changes appear in the same audit log as the GraphQL ones.
// Best-effort: any failure is ignored so it never blocks the command.
func logCLIInvocation(cmd *cobra.Command, args []string) {
	if cliReadOnlyCommands[cmd.Name()] {
		return
	}

	uri := dbURI
	if uri == "" {
		uri = viper.GetString("db.uri")
	}
	sem := semester
	if sem == "" {
		sem = viper.GetString("semester")
	}
	if uri == "" || sem == "" {
		return
	}

	dbName := viper.GetString("db.database")
	var dbNamePtr *string
	if dbName != "" {
		dbNamePtr = &dbName
	}
	dbClient, err := db.NewDB(uri, sem, dbNamePtr)
	if err != nil {
		return
	}
	ctx := context.Background()
	defer dbClient.Client.Disconnect(ctx) //nolint:errcheck

	pairs := make([]*model.MutationLogArg, 0)
	ancodeSet := make(map[int]struct{})
	addPair := func(key, value string) {
		pairs = append(pairs, &model.MutationLogArg{Key: key, Value: value})
		if n, err := strconv.Atoi(value); err == nil {
			ancodeSet[n] = struct{}{}
		}
	}

	for i, a := range args {
		addPair("arg"+strconv.Itoa(i), a)
	}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		addPair(f.Name, f.Value.String())
	})

	ancodes := make([]int, 0, len(ancodeSet))
	for a := range ancodeSet {
		ancodes = append(ancodes, a)
	}

	entry := &model.MutationLogEntry{
		Time:    time.Now(),
		Name:    cmd.Name(),
		Type:    "cli",
		Args:    pairs,
		Ancodes: ancodes,
	}
	_ = dbClient.AddMutationLogEntry(ctx, entry)
}
