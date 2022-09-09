package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/spf13/viper"
)

type Plexams struct {
	semester string
	dbClient *db.DB
	zpa      *ZPA
	workflow []*model.Step
	planer   *Planer
	email    *Email
}

type ZPA struct {
	client                *zpa.ZPA
	baseurl               string
	username              string
	password              string
	studentRegsForProgram []string
}

type Planer struct {
	Name  string
	Email string
}

type Email struct {
	server   string
	port     int
	username string
	password string
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
		workflow: initWorkflow(),
		planer: &Planer{
			Name:  viper.GetString("planer.name"),
			Email: viper.GetString("planer.email"),
		},
		email: &Email{
			server:   viper.GetString("smtp.server.name"),
			port:     viper.GetInt("smtp.server.port"),
			username: viper.GetString("smtp.username"),
			password: viper.GetString("smtp.password"),
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

func (p *Plexams) Log(ctx context.Context, subj, msg string) error {
	return p.dbClient.Log(ctx, subj, msg)
}
