package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PrimussExams(ctx context.Context) ([]*model.PrimussExamByProgram, error) {
	byProgram, err := p.dbClient.GetPrimussExams(ctx)
	if err != nil {
		return nil, err
	}
	// mark the connection status authoritatively: a Primuss exam is connected iff it is
	// claimed by some ZPA exam (incl. manually added ancodes) or an external (MUC.DAI)
	// exam — the same source the connected-exams view uses.
	claimed, err := p.claimedPrimussKeys(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot compute claimed primuss keys for connection status")
		return byProgram, nil
	}
	plannedZPA, err := p.toPlanZPAPrimussKeys(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot compute to-plan ZPA primuss keys")
		plannedZPA = map[primussKey]bool{}
	}
	for _, prog := range byProgram {
		for _, exam := range prog.Exams {
			key := primussKey{exam.Program, exam.Ancode}
			exam.Connected = claimed[key]
			exam.PlannedZpa = plannedZPA[key]
		}
	}
	return byProgram, nil
}

// toPlanZPAPrimussKeys returns the (program, ancode) of every Primuss exam claimed by a
// ZPA exam selected to be planned (a subset of claimedPrimussKeys: excludes external /
// MUC.DAI exams and ZPA exams marked not-to-plan).
func (p *Plexams) toPlanZPAPrimussKeys(ctx context.Context) (map[primussKey]bool, error) {
	toPlan, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		return nil, err
	}
	keys := make(map[primussKey]bool)
	for _, e := range toPlan {
		for _, pa := range e.PrimussAncodes {
			if pa.Ancode > 0 {
				keys[primussKey{pa.Program, pa.Ancode}] = true
			}
		}
	}
	return keys, nil
}

func (p *Plexams) GetPrimussExam(ctx context.Context, program string, ancode int) (*model.PrimussExam, error) {
	return p.dbClient.GetPrimussExam(ctx, program, ancode)
}

func (p *Plexams) PrimussExamExists(ctx context.Context, program string, ancode int) (bool, error) {
	return p.dbClient.PrimussExamExists(ctx, program, ancode)
}

func (p *Plexams) GetPrimussExamsForAncode(ctx context.Context, ancode int) ([]*model.PrimussExam, error) {
	return p.dbClient.GetPrimussExamsForAncode(ctx, ancode)
}

func (p *Plexams) GetStudentRegs(ctx context.Context, exam *model.PrimussExam) ([]*model.StudentReg, error) {
	return p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, exam.Program, exam.AnCode)
}

func (p *Plexams) GetEnhancedStudentRegs(ctx context.Context, program string, ancode int) ([]*model.EnhancedStudentReg, error) {
	studentRegs, err := p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, program, ancode)
	if err != nil {
		return nil, err
	}

	enhancedStudentRegs := make([]*model.EnhancedStudentReg, 0, len(studentRegs))
	for _, studentReg := range studentRegs {
		zpaStudent, err := p.dbClient.GetZPAStudentByMtknr(ctx, studentReg.Mtknr)
		if err != nil {
			log.Debug().Err(err).Str("mtknr", studentReg.Mtknr).Msg("cannot find zpa student for student reg")
		}
		enhancedStudentRegs = append(enhancedStudentRegs, &model.EnhancedStudentReg{
			ZpaStudent: zpaStudent,
			Mtknr:      studentReg.Mtknr,
			Ancode:     studentReg.AnCode,
			Program:    studentReg.Program,
			Group:      studentReg.Group,
			Name:       studentReg.Name,
			Presence:   studentReg.Presence,
		})
	}
	return enhancedStudentRegs, nil
}

func (p *Plexams) StudentRegsForProgram(ctx context.Context, program string) ([]*model.StudentReg, error) {
	return p.dbClient.StudentRegsForProgram(ctx, program)
}

func (p *Plexams) StudentRegsPerStudentPlanned(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.StudentRegsPerStudentPlanned(ctx)
}

func (p *Plexams) StudentRegsImportErrors(ctx context.Context) ([]*model.RegWithError, error) {
	return p.dbClient.GetRegsWithErrors(ctx)
}

func (p *Plexams) GetConflicts(ctx context.Context, exam *model.PrimussExam) (*model.Conflicts, error) {
	return p.dbClient.GetPrimussConflictsForAncode(ctx, exam.Program, exam.AnCode)
}

func (p *Plexams) AddAncode(ctx context.Context, zpaAncode int, program string, primussAncode int) error {
	return p.dbClient.AddAncode(ctx, zpaAncode, program, primussAncode)
}
