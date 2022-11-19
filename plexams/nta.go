package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AddNta(ctx context.Context, input model.NTAInput) (*model.NTA, error) {
	return p.dbClient.AddNta(ctx, model.NtaInputToNta(input))
}

func (p *Plexams) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return p.dbClient.Ntas(ctx)
}

func (p *Plexams) NtasWithRegs(ctx context.Context) ([]*model.NTAWithRegs, error) {
	return p.dbClient.NtasWithRegs(ctx)
}

func (p *Plexams) Nta(ctx context.Context, mtknr string) (*model.NTAWithRegs, error) {
	return p.dbClient.Nta(ctx, mtknr)
}

func (p *Plexams) PrepareNta() error {
	ctx := context.Background()
	// get NTAs
	ntas, err := p.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get NTAs")
		return err
	}

	// get StudentRegs
	studentRegs, err := p.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs")
		return err
	}

	// merge
	ntaWithRegs := make([]*model.NTAWithRegs, 0)
	for _, nta := range ntas {
		for _, studentReg := range studentRegs {
			if nta.Mtknr == studentReg.Student.Mtknr {
				ntaWithRegs = append(ntaWithRegs, &model.NTAWithRegs{
					Nta:  nta,
					Regs: studentReg,
				})
				break
			}
		}
	}

	err = p.dbClient.SaveSemesterNTAs(ctx, ntaWithRegs)
	if err != nil {
		log.Error().Err(err).Msg("cannot save ntas for semester")
		return err
	}

	return nil
}
