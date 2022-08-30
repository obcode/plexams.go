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
	dbClient *db.DB
	zpa      *ZPA
}

type ZPA struct {
	client                *zpa.ZPA
	baseurl               string
	username              string
	password              string
	studentRegsForProgram []string
}

func NewPlexams(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword string, studentRegsForProgram []string) (*Plexams, error) {
	client, err := db.NewDB(dbUri, semester)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
	}

	return &Plexams{
		semester: semester,
		dbClient: client,
		zpa: &ZPA{
			client:                nil,
			baseurl:               zpaBaseurl,
			username:              zpaUsername,
			password:              zpaPassword,
			studentRegsForProgram: studentRegsForProgram,
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

func (p *Plexams) GetSemester(ctx context.Context) *model.Semester {
	return &model.Semester{
		ID: p.semester,
	}
}

func (p *Plexams) SetSemester(ctx context.Context, s string) (*model.Semester, error) {
	p.semester = s
	err := p.dbClient.SetSemester(s)
	if err != nil {
		return nil, err
	}
	return &model.Semester{
		ID: p.semester,
	}, nil
}

func (p *Plexams) Log(ctx context.Context, msg string) error {
	return p.dbClient.Log(ctx, msg)
}
