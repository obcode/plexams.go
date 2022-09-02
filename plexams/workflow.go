package plexams

import (
	"context"

	"github.com/gookit/color"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func initWorkflow() []*model.Step {
	var workflow []*model.Step
	err := viper.UnmarshalKey("workflow", &workflow)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot decode workflow with viper")
	}

	for i, s := range workflow {
		s.Number = i + 1
	}

	return workflow
}

func (p *Plexams) GetWorkflow(ctx context.Context) ([]*model.Step, error) {
	return p.workflow, nil
}

func (p *Plexams) PrintWorkflow() {
	for _, step := range p.workflow {
		if step.Done {
			color.Green.Printf("%2d. %s\n", step.Number, step.Name)
		} else {
			color.Red.Printf("%2d. %s\n", step.Number, step.Name)
		}
	}
}
