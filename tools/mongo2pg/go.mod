// Own module on purpose: this one-off needs the MongoDB driver, and the main
// module just got rid of it. Deleted together with this tool after the cut-over.
module github.com/obcode/plexams.go/tools/mongo2pg

go 1.25.7

require (
	github.com/obcode/plexams.go v0.0.0
	go.mongodb.org/mongo-driver v1.17.4
)

require (
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.23 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/pressly/goose/v3 v3.27.3 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/sethvargo/go-retry v0.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/obcode/plexams.go => ../..
