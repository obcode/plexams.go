package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetTeachers(ctx context.Context, fromZpa *bool) ([]*model.Teacher, error) {
	if fromZpa != nil && *fromZpa {
		if err := p.SetZPA(); err != nil {
			return nil, err
		}

		teachers := p.zpa.client.GetTeachers()

		err := p.dbClient.CacheTeachers(teachers, p.semester)
		if err != nil {
			return nil, err
		}
		return teachers, nil
	} else {
		return p.dbClient.GetTeachers(ctx)
	}
}

func (p *Plexams) GetInvigilators(ctx context.Context) ([]*model.Teacher, error) {
	return p.dbClient.GetInvigilators(ctx)
}
