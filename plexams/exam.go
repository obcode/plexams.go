package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) AddAdditionalExam(ctx context.Context, exam model.AdditionalExamInput) (bool, error) {
	return p.dbClient.AddAdditionalExam(ctx, exam)
}

func (p *Plexams) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	return p.dbClient.AdditionalExams(ctx)
}

func (p *Plexams) GetConnectedExam(ctx context.Context, anCode int, allPrograms []string) (*model.ConnectedExam, error) {
	zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, anCode)
	if err != nil {
		return nil, err
	}

	allKeys := make(map[string]bool)
	programs := []string{}
	for _, group := range zpaExam.Groups {
		program := group[:2]
		if _, value := allKeys[program]; !value {
			allKeys[program] = true
			programs = append(programs, program)
		}
	}

	primussExams := make([]*model.PrimussExam, 0)
	var errors []string

	for _, program := range programs {
		primussExam, err := p.GetPrimussExam(ctx, program, anCode)
		if err != nil {
			if errors == nil {
				errors = make([]string, 0)
			}
			errors = append(errors, fmt.Sprintf("%s/%d not found", program, anCode))
		} else {
			primussExams = append(primussExams, primussExam)
		}
	}

	otherPrograms := make([]string, 0, len(allPrograms)-len(programs))
OUTER:
	for _, aP := range allPrograms {
		for _, p := range programs {
			if aP == p {
				continue OUTER
			}
		}
		otherPrograms = append(otherPrograms, aP)
	}

	var otherPrimussExams []*model.PrimussExam

	for _, program := range otherPrograms {
		primussExam, err := p.GetPrimussExam(ctx, program, anCode)
		if err == nil {
			if otherPrimussExams == nil {
				otherPrimussExams = make([]*model.PrimussExam, 0)
			}
			if errors == nil {
				errors = make([]string, 0)
			}
			errors = append(errors, fmt.Sprintf("found %s/%d (%s: %s)", program, anCode, primussExam.MainExamer, primussExam.Module))
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

func (p *Plexams) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	return p.dbClient.GetConnectedExams(ctx)
}

func (p *Plexams) PrepareConnectedExams() error {
	ctx := context.Background()
	anCodes, err := p.GetZpaAnCodes(ctx)
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
	for _, anCode := range anCodes {
		exam, err := p.GetConnectedExam(ctx, anCode.AnCode, allPrograms)
		if err != nil {
			log.Error().Err(err).Int("ancode", anCode.AnCode).
				Msg("cannot connected exam")
			return err
		}
		exams = append(exams, exam)
	}

	err = p.dbClient.SaveConnectedExams(ctx, exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot save connected exams")
		return err
	}

	return nil
}

func (p *Plexams) PrepareExams(ctx context.Context, inputs []*model.PrimussExamInput) (bool, error) {
	if p.dbClient.ExamsAlreadyPrepared(ctx) {
		oks := true
		for _, input := range inputs {
			ok, err := p.RemovePrimussExam(ctx, input)
			if err != nil {
				return false, err
			}
			oks = oks && ok
		}
		return oks, nil
	} else {
		connectedExams, err := p.GetConnectedExams(ctx)
		if err != nil {
			return false, err
		}

		oks := true
		for _, connectedExam := range connectedExams {
			// generate Exam and add Teacher
			exam, err := p.zpaExamToExam(ctx, connectedExam.ZpaExam)
			if err != nil {
				// FIXME: Maybe not a good idea?
				return false, err
			}

			for _, primussExam := range connectedExam.PrimussExams {

				if isConnected(primussExam, inputs) {
					studentRegs, err := p.GetStudentRegs(ctx, primussExam)
					if err != nil {
						err := p.Log(ctx, fmt.Sprintf("no studentRegs for primuss exam %s/%d",
							primussExam.Program, primussExam.AnCode), "")
						if err != nil {
							log.Error().Err(err).Msg("cannot log")
						}
					}
					conflicts, err := p.GetConflicts(ctx, primussExam)
					if err != nil {
						err := p.Log(ctx, fmt.Sprintf("no studentRegs for primuss exam %s/%d",
							primussExam.Program, primussExam.AnCode), "")
						if err != nil {
							log.Error().Err(err).Msg("cannot log")
						}
					}
					exam.RegisteredExams = append(exam.RegisteredExams, &model.RegisteredExam{
						Exam:        primussExam,
						StudentRegs: studentRegs,
						Conflicts:   conflicts.Conflicts,
					})
				} else { // should not be connected
					if exam.RemovedPrimussExams == nil {
						exam.RemovedPrimussExams = make([]model.RemovedPrimussExam, 0)
					}
					exam.RemovedPrimussExams = append(exam.RemovedPrimussExams,
						model.RemovedPrimussExam{
							AnCode:  primussExam.AnCode,
							Program: primussExam.Program,
						})
					// log to MongoDb
					err := p.Log(ctx, fmt.Sprintf("removed primuss exam %s/%d from exam %d",
						primussExam.Program, primussExam.AnCode, exam.AnCode), "")
					if err != nil {
						log.Error().Err(err).Str("program", primussExam.Program).
							Int("anCode", primussExam.AnCode).
							Msg("cannot log removed primuss exam")
					}
				}
			}

			// add exam to db
			err = p.dbClient.AddExam(ctx, exam)
			if err != nil {
				log.Error().Err(err).Int("anCode", exam.AnCode).Msg("cannot insert exam to db")
			}
		}

		return oks, nil
	}
}

func (p *Plexams) RemovePrimussExam(ctx context.Context, input *model.PrimussExamInput) (bool, error) {
	// TODO: Implement me
	// wenn schon in DB, dann einzelne Prüfung herausnehmen und updaten
	// if true {
	// 	oks := true
	// 	for _, input := range input {
	// 		ok, err := p.RemovePrimussExam(ctx, *input)
	// 		oks = oks && ok
	// 		if err != nil {
	// 			log.Error().Err(err).
	// 				Int("anCode", input.AnCode).Str("program", input.Program).
	// 				Msg("cannot remove primuss exam")
	// 			return oks, err
	// 		}
	// 	}
	// 	return oks, nil
	// }
	return true, nil
}

func isConnected(primussExam *model.PrimussExam, notConnectedExams []*model.PrimussExamInput) bool {
	for _, notConnectedExam := range notConnectedExams {
		if primussExam.AnCode == notConnectedExam.AnCode && primussExam.Program == notConnectedExam.Program {
			return false
		}
	}

	return true
}

func (p *Plexams) zpaExamToExam(ctx context.Context, zpaExam *model.ZPAExam) (*model.Exam, error) {
	mainExamer, err := p.dbClient.GetTeacher(ctx, zpaExam.MainExamerID)
	if err != nil {
		log.Error().Err(err).Int("AnCode", zpaExam.AnCode).Int("MainExamerID", zpaExam.MainExamerID).
			Str("MainExamerName", zpaExam.MainExamer).Msg("cannot find main examer")
		return nil, err
	}

	return &model.Exam{
		Semester:            zpaExam.Semester,
		AnCode:              zpaExam.AnCode,
		Module:              zpaExam.Module,
		MainExamer:          mainExamer,
		MainExamerName:      zpaExam.MainExamer,
		MainExamerID:        zpaExam.MainExamerID,
		ExamType:            zpaExam.ExamType,
		Duration:            zpaExam.Duration,
		IsRepeaterExam:      zpaExam.IsRepeaterExam,
		ZpaGroups:           zpaExam.Groups,
		RemovedPrimussExams: nil,
		RegisteredExams:     []*model.RegisteredExam{},
	}, nil
}
