package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) GetZPAExams(ctx context.Context, fromZpa *bool) ([]*model.ZPAExam, error) {
	if fromZpa != nil && *fromZpa {
		if err := p.SetZPA(); err != nil {
			return nil, err
		}

		exams := p.zpa.client.GetExams()

		err := p.dbClient.CacheZPAExams(exams, p.semester)
		if err != nil {
			return nil, err
		}
		return exams, nil
	} else {
		return p.dbClient.GetZPAExams(ctx)
	}
}
