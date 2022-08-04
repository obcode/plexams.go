package graph

//go:generate go run github.com/99designs/gqlgen generate

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
)

type PlexamsResolver struct {
	plexams *plexams.Plexams
}

func NewPlexamsResolver(uri string) *PlexamsResolver {

	plexams, err := plexams.NewPlexams(uri)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	return &PlexamsResolver{
		plexams: plexams,
	}
}

func (r *PlexamsResolver) Query() generated.QueryResolver { return r }

func (r *PlexamsResolver) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return r.plexams.GetAllSemesterNames(ctx)
}
