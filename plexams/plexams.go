package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
)

type Plexams struct {
	semester string
	dbClient *db.Client
	zpa      *ZPA
}

type ZPA struct {
	client   *zpa.ZPA
	baseurl  string
	username string
	password string
}

func NewPlexams(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword string) (*Plexams, error) {
	client, err := db.NewClient(dbUri)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	return &Plexams{
		semester: semester,
		dbClient: client,
		zpa: &ZPA{
			client:   nil,
			baseurl:  zpaBaseurl,
			username: zpaUsername,
			password: zpaPassword,
		},
	}, nil
}

func (p *Plexams) SetZPA() error {
	if p.zpa.client == nil {
		zpaClient, err := zpa.NewZPA(p.zpa.baseurl, p.zpa.username, p.zpa.password, p.semester)
		if err != nil {
			return err
		}
		p.zpa.client = zpaClient
	}
	return nil
}

func (p *Plexams) GetAllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return p.dbClient.AllSemesterNames()
}

func (p *Plexams) GetTeachers(ctx context.Context) ([]*model.Teacher, error) {
	if err := p.SetZPA(); err != nil {
		return nil, err
	}

	teachers := p.zpa.client.GetTeachers()

	err := p.dbClient.CacheTeachers(teachers, p.semester)
	if err != nil {
		return nil, err
	}

	return teachers, nil
}
