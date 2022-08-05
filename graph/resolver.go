package graph

//go:generate go run github.com/99designs/gqlgen generate

import (
	"fmt"

	"github.com/obcode/plexams.go/plexams"
)

type Resolver struct {
	plexams *plexams.Plexams
}

func NewResolver(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword string) *Resolver {

	plexams, err := plexams.NewPlexams(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	return &Resolver{
		plexams: plexams,
	}
}
