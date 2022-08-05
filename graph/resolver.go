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

func NewPlexamsResolver(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword string) *PlexamsResolver {

	plexams, err := plexams.NewPlexams(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword)

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

func (r *PlexamsResolver) Teachers(ctx context.Context, fromZpa *bool) ([]*model.Teacher, error) {
	return r.plexams.GetTeachers(ctx, fromZpa)
}

func (r *PlexamsResolver) Invigilators(ctx context.Context) ([]*model.Teacher, error) {
	return r.plexams.GetInvigilators(ctx)
}
