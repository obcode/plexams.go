package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ExamerInPlan(ctx context.Context) ([]*model.ExamerInPlan, error) {
	return p.dbClient.ExamerInPlan(ctx)
}

func (p *Plexams) prepareConnectedZPAExam(ctx context.Context, ancode int, allPrograms []string) (*model.ConnectedExam, error) {
	zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		return nil, err
	}

	primussExams := make([]*model.PrimussExam, 0)
	errors := make([]string, 0)

	// Replace with primuss ancodes
	for _, primussAncode := range zpaExam.PrimussAncodes {
		if primussAncode.Ancode == 0 || primussAncode.Ancode == -1 {
			continue
		}

		// Skip old programs
		skipProgram := false
		for _, oldProgram := range p.zpa.oldprograms {
			if primussAncode.Program == oldProgram {
				skipProgram = true
				break
			}
		}
		if skipProgram {
			continue
		}

		primussExam, err := p.GetPrimussExam(ctx, primussAncode.Program, primussAncode.Ancode)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s/%d not found", primussAncode.Program, primussAncode.Ancode))
		} else {
			primussExams = append(primussExams, primussExam)
		}
	}

	otherPrograms := make([]string, 0, len(allPrograms)-len(zpaExam.PrimussAncodes))
OUTER:
	for _, aP := range allPrograms {
		for _, p := range zpaExam.PrimussAncodes {
			if aP == p.Program {
				continue OUTER
			}
		}
		otherPrograms = append(otherPrograms, aP)
	}

	var otherPrimussExams []*model.PrimussExam

	for _, program := range otherPrograms {
		primussExam, err := p.GetPrimussExam(ctx, program, ancode)
		if err == nil {
			if otherPrimussExams == nil {
				otherPrimussExams = make([]*model.PrimussExam, 0)
			}
			errors = append(errors, fmt.Sprintf("found %s/%d (%s: %s)", program, ancode, primussExam.MainExamer, primussExam.Module))
			otherPrimussExams = append(otherPrimussExams, primussExam)
		}
	}

	return &model.ConnectedExam{
		ZpaExam:           zpaExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: otherPrimussExams,
		Errors:            errors,
	}, nil
}

func (p *Plexams) prepareConnectedNonZPAExam(ctx context.Context, ancode int, nonZPAExam *model.ZPAExam) (*model.ConnectedExam, error) {
	if nonZPAExam == nil {
		var err error
		nonZPAExam, err = p.dbClient.NonZpaExam(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get non zpa exam by ancode")
			return nil, err
		}
	}

	primussExams := make([]*model.PrimussExam, 0)
	for _, primuss := range nonZPAExam.PrimussAncodes {
		primussExam, err := p.GetPrimussExam(ctx, primuss.Program, primuss.Ancode)
		if err != nil {
			log.Error().Err(err).Str("program", primuss.Program).Int("ancode", primuss.Ancode).
				Msg("cannot get primuss exam")
			return nil, err
		}
		primussExams = append(primussExams, primussExam)
	}

	return &model.ConnectedExam{
		ZpaExam:           nonZPAExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: nil,
		Errors:            []string{},
	}, nil
}

func (p *Plexams) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	return p.dbClient.GetConnectedExams(ctx)
}

func (p *Plexams) GetConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	return p.dbClient.GetConnectedExam(ctx, ancode)
}

func (p *Plexams) PrepareConnectedExams() error {
	ctx := context.Background()
	ancodes, err := p.GetZpaAnCodesToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa ancodes")
		return err
	}

	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return err
	}

	exams := make([]*model.ConnectedExam, 0)
	for _, ancode := range ancodes {
		exam, err := p.prepareConnectedZPAExam(ctx, ancode.Ancode, allPrograms)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode.Ancode).
				Msg("cannot connected exam")
			return err
		}
		exams = append(exams, exam)
	}

	nonZPAExams, err := p.dbClient.NonZpaExams(ctx)
	if err == nil {
		for _, nonZPAExam := range nonZPAExams {
			connectedExam, err := p.prepareConnectedNonZPAExam(ctx, nonZPAExam.AnCode, nonZPAExam)
			if err != nil {
				log.Error().Err(err).Int("ancode", nonZPAExam.AnCode).
					Msg("cannot prepare connected non zpa exam")
				return err
			}
			exams = append(exams, connectedExam)
		}
	}

	err = p.dbClient.SaveConnectedExams(ctx, exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot save connected exams")
		return err
	}

	return nil
}

func (p *Plexams) PrepareConnectedExam(ancode int) error {
	ctx := context.Background()

	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return err
	}

	var exam *model.ConnectedExam
	if ancode < 1000 {
		exam, err = p.prepareConnectedZPAExam(ctx, ancode, allPrograms)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).
				Msg("cannot connected exam")
			return err
		}
	} else {
		exam, err = p.prepareConnectedNonZPAExam(ctx, ancode, nil)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get non zpa exam by ancode")
			return err
		}
	}

	if exam != nil {
		err = p.dbClient.SaveConnectedExam(ctx, exam)
		if err != nil {
			log.Error().Err(err).Msg("cannot save connected exam")
			return err
		}
	}

	return nil
}

// TODO: check if there are Exams with same Ancode in other programs
func (p *Plexams) ConnectExam(ancode int, program string) error {
	ctx := context.Background()
	connectedExam, err := p.GetConnectedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get connected exam")
		return err
	}

	for _, primussExam := range connectedExam.PrimussExams {
		if primussExam.AnCode == ancode && primussExam.Program == program {
			log.Debug().Msg("primuss exam already connected")
			return fmt.Errorf("primuss exam already connected")
		}
	}

	primussExam, err := p.GetPrimussExam(ctx, program, ancode)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get primuss exam")
		return err
	}

	connectedExam.PrimussExams = append(connectedExam.PrimussExams, primussExam)

	if len(connectedExam.OtherPrimussExams) > 0 {
		otherPrimussExams := make([]*model.PrimussExam, 0)
		for _, exam := range connectedExam.OtherPrimussExams {
			if exam.AnCode != ancode || exam.Program != program {
				otherPrimussExams = append(otherPrimussExams, exam)
			}
			if len(otherPrimussExams) > 0 {
				connectedExam.OtherPrimussExams = otherPrimussExams
			} else {
				connectedExam.OtherPrimussExams = nil
			}
		}
	}

	return p.dbClient.ReplaceConnectedExam(ctx, connectedExam)
}
