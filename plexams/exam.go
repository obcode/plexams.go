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

func (p *Plexams) prepareConnectedExam(ctx context.Context, ancode int, allPrograms []string) (*model.ConnectedExam, error) {
	zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
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
		primussExam, err := p.GetPrimussExam(ctx, program, ancode)
		if err != nil {
			if errors == nil {
				errors = make([]string, 0)
			}
			errors = append(errors, fmt.Sprintf("%s/%d not found", program, ancode))
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
		primussExam, err := p.GetPrimussExam(ctx, program, ancode)
		if err == nil {
			if otherPrimussExams == nil {
				otherPrimussExams = make([]*model.PrimussExam, 0)
			}
			if errors == nil {
				errors = make([]string, 0)
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
		exam, err := p.prepareConnectedExam(ctx, ancode.Ancode, allPrograms)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode.Ancode).
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

func (p *Plexams) PrepareConnectedExam(ancode int) error {
	ctx := context.Background()

	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return err
	}

	exam, err := p.prepareConnectedExam(ctx, ancode, allPrograms)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).
			Msg("cannot connected exam")
		return err
	}

	err = p.dbClient.SaveConnectedExam(ctx, exam)
	if err != nil {
		log.Error().Err(err).Msg("cannot save connected exam")
		return err
	}

	return nil
}

// func (p *Plexams) PrepareExams(ctx context.Context, inputs []*model.PrimussExamInput) (bool, error) {
// 	if p.dbClient.ExamsAlreadyPrepared(ctx) {
// 		oks := true
// 		for _, input := range inputs {
// 			ok, err := p.RemovePrimussExam(ctx, input)
// 			if err != nil {
// 				return false, err
// 			}
// 			oks = oks && ok
// 		}
// 		return oks, nil
// 	} else {
// 		connectedExams, err := p.GetConnectedExams(ctx)
// 		if err != nil {
// 			return false, err
// 		}

// 		oks := true
// 		for _, connectedExam := range connectedExams {
// 			// generate Exam and add Teacher
// 			exam, err := p.zpaExamToExam(ctx, connectedExam.ZpaExam)
// 			if err != nil {
// 				// FIXME: Maybe not a good idea?
// 				return false, err
// 			}

// 			for _, primussExam := range connectedExam.PrimussExams {

// 				if isConnected(primussExam, inputs) {
// 					studentRegs, err := p.GetStudentRegs(ctx, primussExam)
// 					if err != nil {
// 						err := p.Log(ctx, fmt.Sprintf("no studentRegs for primuss exam %s/%d",
// 							primussExam.Program, primussExam.AnCode), "")
// 						if err != nil {
// 							log.Error().Err(err).Msg("cannot log")
// 						}
// 					}
// 					conflicts, err := p.GetConflicts(ctx, primussExam)
// 					if err != nil {
// 						err := p.Log(ctx, fmt.Sprintf("no studentRegs for primuss exam %s/%d",
// 							primussExam.Program, primussExam.AnCode), "")
// 						if err != nil {
// 							log.Error().Err(err).Msg("cannot log")
// 						}
// 					}
// 					exam.RegisteredExams = append(exam.RegisteredExams, &model.RegisteredExam{
// 						Exam:        primussExam,
// 						StudentRegs: studentRegs,
// 						Conflicts:   conflicts.Conflicts,
// 					})
// 				} else { // should not be connected
// 					if exam.RemovedPrimussExams == nil {
// 						exam.RemovedPrimussExams = make([]model.RemovedPrimussExam, 0)
// 					}
// 					exam.RemovedPrimussExams = append(exam.RemovedPrimussExams,
// 						model.RemovedPrimussExam{
// 							AnCode:  primussExam.AnCode,
// 							Program: primussExam.Program,
// 						})
// 					// log to MongoDb
// 					err := p.Log(ctx, fmt.Sprintf("removed primuss exam %s/%d from exam %d",
// 						primussExam.Program, primussExam.AnCode, exam.AnCode), "")
// 					if err != nil {
// 						log.Error().Err(err).Str("program", primussExam.Program).
// 							Int("ancode", primussExam.AnCode).
// 							Msg("cannot log removed primuss exam")
// 					}
// 				}
// 			}

// 			// add exam to db
// 			err = p.dbClient.AddExam(ctx, exam)
// 			if err != nil {
// 				log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot insert exam to db")
// 			}
// 		}

// 		return oks, nil
// 	}
// }

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
	// 				Int("ancode", input.AnCode).Str("program", input.Program).
	// 				Msg("cannot remove primuss exam")
	// 			return oks, err
	// 		}
	// 	}
	// 	return oks, nil
	// }
	return true, nil
}

// func isConnected(primussExam *model.PrimussExam, notConnectedExams []*model.PrimussExamInput) bool {
// 	for _, notConnectedExam := range notConnectedExams {
// 		if primussExam.AnCode == notConnectedExam.Ancode && primussExam.Program == notConnectedExam.Program {
// 			return false
// 		}
// 	}

// 	return true
// }

// func (p *Plexams) zpaExamToExam(ctx context.Context, zpaExam *model.ZPAExam) (*model.Exam, error) {
// 	mainExamer, err := p.dbClient.GetTeacher(ctx, zpaExam.MainExamerID)
// 	if err != nil {
// 		log.Error().Err(err).Int("AnCode", zpaExam.AnCode).Int("MainExamerID", zpaExam.MainExamerID).
// 			Str("MainExamerName", zpaExam.MainExamer).Msg("cannot find main examer")
// 		return nil, err
// 	}

// 	return &model.Exam{
// 		Semester:            zpaExam.Semester,
// 		AnCode:              zpaExam.AnCode,
// 		Module:              zpaExam.Module,
// 		MainExamer:          mainExamer,
// 		MainExamerName:      zpaExam.MainExamer,
// 		MainExamerID:        zpaExam.MainExamerID,
// 		ExamType:            zpaExam.ExamType,
// 		Duration:            zpaExam.Duration,
// 		IsRepeaterExam:      zpaExam.IsRepeaterExam,
// 		ZpaGroups:           zpaExam.Groups,
// 		RemovedPrimussExams: nil,
// 		RegisteredExams:     []*model.RegisteredExam{},
// 	}, nil
// }

func (p *Plexams) ExamWithRegs(ctx context.Context, ancode int) (*model.ExamWithRegs, error) {
	return p.dbClient.ExamWithRegs(ctx, ancode)
}

func (p *Plexams) ExamsWithRegs(ctx context.Context) ([]*model.ExamWithRegs, error) {
	return p.dbClient.ExamsWithRegs(ctx)
}

func (p *Plexams) ExamGroup(ctx context.Context, examGroupCode int) (*model.ExamGroup, error) {
	return p.dbClient.ExamGroup(ctx, examGroupCode)
}

func (p *Plexams) ExamGroups(ctx context.Context) ([]*model.ExamGroup, error) {
	return p.dbClient.ExamGroups(ctx)
}

func (p *Plexams) ConflictingGroupCodes(ctx context.Context, examGroupCode int) ([]*model.ExamGroupConflict, error) {
	examGroup, err := p.ExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).Msg("cannot get exam group")
		return nil, err
	}
	return examGroup.ExamGroupInfo.Conflicts, nil
}
