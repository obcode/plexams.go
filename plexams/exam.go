package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// Deprecated: rm me
func (p *Plexams) AddAdditionalExam(ctx context.Context, exam model.AdditionalExamInput) (bool, error) {
	return p.dbClient.AddAdditionalExam(ctx, exam)
}

// Deprecated: rm me
func (p *Plexams) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	return p.dbClient.AdditionalExams(ctx)
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

func (p *Plexams) Exam(ctx context.Context, ancode int) (*model.Exam, error) {
	connectedExam, err := p.GetConnectedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get connected exam")
		return nil, err
	}
	// TODO: maybe external exam?

	studentRegs := make([]*model.StudentRegsPerAncodeAndProgram, 0, len(connectedExam.PrimussExams))
	// TODO: only conflicts of planned primussExams?
	conflicts := make([]*model.ConflictsPerProgramAncode, 0, len(connectedExam.PrimussExams))

	for _, primussExam := range connectedExam.PrimussExams {
		studentRegsProgram, err := p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, primussExam.Program, primussExam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", primussExam.AnCode).Str("program", primussExam.Program).Msg("cannot get studentregs")
			return nil, err
		}
		studentRegs = append(studentRegs, &model.StudentRegsPerAncodeAndProgram{
			Program:     primussExam.Program,
			Ancode:      primussExam.AnCode,
			StudentRegs: studentRegsProgram,
		})

		conflictsProgram, err := p.dbClient.GetPrimussConflictsForAncode(ctx, primussExam.Program, primussExam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", primussExam.AnCode).Str("program", primussExam.Program).Msg("cannot get studentregs")
			return nil, err
		}
		conflicts = append(conflicts, &model.ConflictsPerProgramAncode{
			Program:   primussExam.Program,
			Ancode:    primussExam.AnCode,
			Conflicts: conflictsProgram,
		})
	}

	constraints, err := p.ConstraintForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get constraints for ancode")
	}

	regularStuds, ntaStuds, err := p.StudentsFromStudentRegs(ctx, studentRegs)
	if err != nil {
		log.Error().Err(err).Msg("cannot get students from student regs")
	}

	for _, nta := range ntaStuds {
		err := p.dbClient.SetCurrentSemesterOnNTA(ctx, nta.Mtknr)
		if err != nil {
			log.Error().Err(err).Interface("nta", nta).Msg("cannot set current semester on nta")
		}
	}

	// TODO: Maybe make plausibility checks?

	return &model.Exam{
		Ancode:          ancode,
		ZpaExam:         connectedExam.ZpaExam,
		ExternalExam:    nil,
		PrimussExams:    connectedExam.PrimussExams,
		StudentRegs:     studentRegs,
		Conflicts:       conflicts,
		ConnectErrors:   connectedExam.Errors,
		Constraints:     constraints,
		RegularStudents: regularStuds,
		NtaStudents:     ntaStuds,
		Slot:            nil,
		Rooms:           nil,
	}, nil
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

// Deprecated: Remove me
func (p *Plexams) ExamWithRegs(ctx context.Context, ancode int) (*model.ExamWithRegs, error) {
	return p.dbClient.ExamWithRegs(ctx, ancode)
}

// Deprecated: Remove me
func (p *Plexams) ExamsWithRegs(ctx context.Context) ([]*model.ExamWithRegs, error) {
	return p.dbClient.ExamsWithRegs(ctx)
}

// Deprecated: Remove me
func (p *Plexams) ExamGroup(ctx context.Context, examGroupCode int) (*model.ExamGroup, error) {
	return p.dbClient.ExamGroup(ctx, examGroupCode)
}

// FIXME: Remove me
func (p *Plexams) ExamGroups(ctx context.Context) ([]*model.ExamGroup, error) {
	return p.dbClient.ExamGroups(ctx)
}

// Deprecated: Remove me
func (p *Plexams) ConflictingGroupCodes(ctx context.Context, examGroupCode int) ([]*model.ExamGroupConflict, error) {
	examGroup, err := p.ExamGroup(ctx, examGroupCode)
	if err != nil {
		log.Error().Err(err).Int("examGroupCode", examGroupCode).Msg("cannot get exam group")
		return nil, err
	}
	return examGroup.ExamGroupInfo.Conflicts, nil
}
