package graph

//go:generate go run github.com/99designs/gqlgen generate

import (
	"github.com/obcode/plexams.go/plexams"
)

type Resolver struct {
	plexams *plexams.Plexams
}

func NewResolver(plexams *plexams.Plexams) *Resolver {
	return &Resolver{
		plexams: plexams,
	}
}
