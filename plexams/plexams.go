package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
)

type Plexams struct {
	dbClient *db.Client
}

func NewPlexams(uri string) (*Plexams, error) {
	client, err := db.NewClient(uri)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	return &Plexams{
		dbClient: client,
	}, nil
}

func (r *Plexams) GetAllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return r.dbClient.AllSemesterNames()
}
